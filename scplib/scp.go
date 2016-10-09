package scplib

import (
	"io/ioutil"
	"os"

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

func (c *Client) UploadDir(local, remote string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendDir(local, remote, nil)

	return errors.Wrap(err, "upload dir")
}

func (c *Client) UploadFile(local, remote string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendFile(local, remote)

	return errors.Wrap(err, "upload file")
}

func (c *Client) Upload(context []byte, remote string) error {
	local, err := ioutil.TempFile("", "go-scp-UploadFile-local")
	if err != nil {
		return errors.Wrap(err, "create tempFile")
	}
	defer os.Remove(local.Name())

	_, err = local.Write(context)
	if err != nil {
		return errors.Wrap(err, "write to file")
	}

	local.Close()

	return c.UploadFile(local.Name(), remote)
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
