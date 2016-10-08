package scplib

import (
	"github.com/hnakamur/go-scp"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

type Client struct {
	c *ssh.Client
}

func NewClient(addr, user, password string) (*Client, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.Password(password)},
	}

	c, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, errors.Wrap(err, "new SSH client")
	}

	return &Client{c}, nil
}

func (c *Client) SendDir(local, remote string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendDir(local, remote, nil)

	return errors.Wrap(err, "ssh send dir")
}

func (c *Client) SendFile(local, remote string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendFile(local, remote)

	return errors.Wrap(err, "ssh send file")
}

func (c *Client) RemoteExec(cmd string) (int, []byte, error) {
	session, err := c.c.NewSession()
	if err != nil {
		return 0, nil, errors.Wrap(err, "ssh client new session")
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)

	if err != nil {
		exitStatus := -1
		exitErr, ok := err.(*ssh.ExitError)
		if ok {
			exitStatus = exitErr.ExitStatus()
		}

		return exitStatus, out, errors.Wrap(err, "ssh command run error")
	}

	return 0, out, nil
}

func (c *Client) Close() error {
	err := c.c.Close()

	return errors.Wrap(err, "close ssh client error")
}
