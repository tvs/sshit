package sshit

import (
	"bufio"
	"context"
	"io"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

type Client struct {
	Config *ssh.ClientConfig
	Server Endpoint

	ctx    context.Context
	cancel context.CancelFunc
	client *ssh.Client
}

type errTimeout struct{}

func (e errTimeout) Error() string { return "i/o timeout" }

var ErrTimeout error = &errTimeout{}

func (c *Client) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	var err error
	c.client, err = ssh.Dial("tcp", c.Server.Address(), c.Config)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}

	return c.client.Close()
}

func (c *Client) Run(cmd string) (string, string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	outPipe, err := session.StdoutPipe()
	if err != nil {
		return "", "", err
	}

	errPipe, err := session.StderrPipe()
	if err != nil {
		return "", "", err
	}

	err = session.Start(cmd)
	if err != nil {
		return "", "", err
	}

	sessDoneChan := make(chan error)
	go func() {
		sessDoneChan <- session.Wait()
	}()

	tChan := time.After(c.Config.Timeout)
	stdoutChan, stderrChan, readDoneChan := read(outPipe, errPipe)

	var stdoutStr, stderrStr string
loop:
	for {
		select {
		case <-tChan:
			return "", "", ErrTimeout
		case err := <-readDoneChan:
			if err != nil {
				return "", "", err
			}
			// Everything is shoved into the channel so we can move on
			break loop
		case o, ok := <-stdoutChan:
			if !ok {
				stdoutChan = nil
			}
			if o != "" {
				stdoutStr += o
			}
		case o, ok := <-stderrChan:
			if !ok {
				stderrChan = nil
			}
			if o != "" {
				stderrStr += o
			}
		}
	}

	select {
	case err := <-sessDoneChan:
		if err != nil {
			return "", "", err
		}
		break
	case <-tChan:
		return "", "", ErrTimeout
	}

	return stdoutStr, stderrStr, nil
}

func (c *Client) Copy(source string, target string) error {
	return nil
}

// read begins the process of reading data off the stdout and stderr readers.
// It returns three channels: the output of stdout, the output of stderr, and
// the done channel.
// When reading is complete, the done channel will return either an error if
// there was one, or nil if the reads completed successfully.
func read(stdout, stderr io.Reader) (chan string, chan string, chan error) {
	oChan := make(chan string)
	eChan := make(chan string)
	done := make(chan error)

	wait := errgroup.Group{}
	wait.Go(func() error {
		return readToChannel(stdout, oChan)
	})
	wait.Go(func() error {
		return readToChannel(stderr, eChan)
	})

	go func() {
		done <- wait.Wait()
	}()

	return oChan, eChan, done
}

// readToChannel reads off the reader, line by line (scanning on newlines) and
// writing the content — including the newline — to the channel.
func readToChannel(reader io.Reader, channel chan string) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		channel <- scanner.Text() + "\n"
	}

	return scanner.Err()
}
