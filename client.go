package sshit

import (
	"context"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	Config *ssh.ClientConfig
	Server Endpoint

	ctx    context.Context
	cancel context.CancelFunc
	client *ssh.Client
}

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

func (c *Client) Run(cmd string) error {
	return nil
}

func (c *Client) Copy(source string, target string) error {
	return nil
}
