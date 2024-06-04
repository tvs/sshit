package sshit

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	defer close(sessDoneChan)

	tChan := time.After(c.Config.Timeout)
	stdoutChan, stderrChan, readDoneChan := read(outPipe, errPipe)
	defer close(stdoutChan)
	defer close(stderrChan)
	defer close(readDoneChan)

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
	closed := false

	// TODO(tvs): Deal with timeouts
	session, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer func() {
		if !closed {
			session.Close()
		}
	}()

	s, err := os.Open(source)
	if err != nil {
		return err
	}
	defer s.Close()

	stat, err := s.Stat()
	if err != nil {
		return err
	}

	w, err := session.StdinPipe()
	if err != nil {
		return err
	}
	defer w.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	if err := session.Start(fmt.Sprintf("scp -tr %s", target)); err != nil {
		return fmt.Errorf("unable to initiate SCP: %w", err)
	}

	if err := write(w, s, stdout, stat.Size(), stat.Mode(), target); err != nil {
		return fmt.Errorf("unable to complete write: %w", err)
	}

	if err := session.Close(); err != nil {
		return fmt.Errorf("unable to close SCP session: %w", err)
	}
	closed = true

	if err := session.Wait(); err != nil {
		return fmt.Errorf("error with remote SCP session: %w", err)
	}

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

// write sends the file over the writer using the SCP protocol and returns an
// error if SCP issues one.
func write(writer io.Writer, reader, stdout io.Reader, size int64, mode fs.FileMode, target string) error {
	targetBase := filepath.Base(target)

	_, err := fmt.Fprintln(writer, fmt.Sprintf("C0%o", mode), size, targetBase)
	if err != nil {
		return err
	}

	if err := checkResponse(stdout); err != nil {
		return err
	}

	_, err = io.Copy(writer, reader)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(writer, "\x00")
	if err != nil {
		return err
	}

	if err := checkResponse(stdout); err != nil {
		return err
	}

	return nil
}

func checkResponse(reader io.Reader) error {
	buf := make([]uint8, 1)
	_, err := reader.Read(buf)
	if err != nil {
		return err
	}

	if buf[0] > 0 {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			msg := scanner.Text()
			return fmt.Errorf("%s", msg)
		}
	}

	return nil
}
