package sshit

import (
	"context"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	Config *ssh.ClientConfig
	Server Endpoint

	ctx    context.Context
	cancel context.CancelFunc
	client *ssh.Client
}

func (s *Session) Connect(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	var err error
	s.client, err = ssh.Dial("tcp", s.Server.Address(), s.Config)
	if err != nil {
		return err
	}

	return nil
}

func (s *Session) Close() error {
	if s.cancel != nil {
		s.cancel()
	}

	return s.client.Close()
}

func (s *Session) Run(cmd string) error {
	return nil
}
