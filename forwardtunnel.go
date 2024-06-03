package sshit

import (
	"context"
	"errors"
	"fmt"
	"net"

	"golang.org/x/sync/errgroup"
)

type forwardTunnel struct {
	ctx      context.Context
	cancel   context.CancelFunc
	wait     *errgroup.Group
	local    Endpoint
	remote   Endpoint
	listener net.Listener
}

func NewForwardTunnel(ctx context.Context, local, remote Endpoint) Tunnel {
	return &forwardTunnel{
		local:  local,
		remote: remote,
	}
}

func (t *forwardTunnel) Bind(c *Client) error {
	if c.client == nil {
		return fmt.Errorf("bind failed, session not connected")
	}

	t.ctx, t.cancel = context.WithCancel(c.ctx)
	t.wait, t.ctx = errgroup.WithContext(t.ctx)

	var err error
	t.listener, t.local.Port, err = bind(t.ctx, t, t.wait, net.Listen, c.client.Dial)
	if err != nil {
		return fmt.Errorf("unable to bind tunnel: %w", err)
	}

	return nil
}

func (t *forwardTunnel) Close() (errs []error) {
	t.cancel()
	if err := t.listener.Close(); err != nil {
		// Ignore errors if the connection was already closed
		if errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}

	if t.wait != nil {
		if err := t.wait.Wait(); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (t *forwardTunnel) Local() Endpoint {
	return t.local
}

func (t *forwardTunnel) Remote() Endpoint {
	return t.remote
}
