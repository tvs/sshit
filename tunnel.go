package sshit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"golang.org/x/sync/errgroup"
)

type Tunnel interface {
	Bind(*Session) error
	Close() []error
	Local() Endpoint
	Remote() Endpoint
}

type listenerFunc func(string, string) (net.Listener, error)
type dialerFunc func(string, string) (net.Conn, error)

func bind(ctx context.Context, t Tunnel, wait *errgroup.Group, listener listenerFunc, dialer dialerFunc) (net.Listener, int, error) {
	l, err := listener("tcp", t.Local().Address())
	if err != nil {
		return nil, 0, fmt.Errorf("unable to start listener: %w", err)
	}

	// Fetch the port we just claimed, in case it was random
	port := l.Addr().(*net.TCPAddr).Port

	wait.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
				local, err := l.Accept()
				if err != nil {
					if errors.Is(err, net.ErrClosed) {
						// Listener has been closed so we can cancel out on the next loop
						continue
					}
					return err
				}

				// Close the local listener once we've been cancelled
				wait.Go(func() error {
					<-ctx.Done()
					return local.Close()
				})

				wait.Go(func() error {
					return dial(ctx, t, local, dialer)
				})
			}
		}
	})

	return l, port, nil
}

func dial(ctx context.Context, t Tunnel, local net.Conn, dialer dialerFunc) error {
	remote, err := dialer("tcp", t.Remote().Address())
	if err != nil {
		return err
	}

	dialWait := errgroup.Group{}

	// Close the remote listener once we've been cancelled. This will also
	// cancel the copy funcs later
	dialWait.Go(func() error {
		<-ctx.Done()
		return remote.Close()
	})

	dialWait.Go(func() error {
		return copyConnections(local, remote)
	})

	dialWait.Go(func() error {
		return copyConnections(remote, local)
	})

	return dialWait.Wait()
}

func copyConnections(dst, src net.Conn) error {
	if _, err := io.Copy(dst, src); err != nil {
		// Ignore the close — that's how we're terminating
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}

	return nil
}
