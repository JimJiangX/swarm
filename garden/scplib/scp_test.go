package scplib

import (
	"bytes"
	"os"
	"testing"
)

const (
	sshAddr  = "127.0.0.1:2222"
	username = "vagrant"
	password = "vagrant"
	context  = `
package scplib

import (
	"io/ioutil"
	"net"
	"os"

	"github.com/hnakamur/go-scp"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

const defaultSSHPort = "22"

// Client contains SSH client.
type Client struct {
	c *ssh.Client
}

// NewClient returns a pointer of Client.
func NewClient(addr, user, password string) (*Client, error) {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		if net.ParseIP(addr) != nil {
			addr = net.JoinHostPort(addr, defaultSSHPort)
		} else {
			return nil, errors.Wrap(err, "parse addr error:"+addr)
		}
	}

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

// UploadDir copies files and directories under the local dir
// to the remote dir.
func (c *Client) UploadDir(remote, local string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendDir(local, remote, nil)

	return errors.Wrap(err, "upload dir")
}

// UploadFile copies a single local file to the remote server.
func (c *Client) UploadFile(remote, local string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendFile(local, remote)

	return errors.Wrap(err, "upload file")
}

// Upload upload string to the remote server.
func (c *Client) Upload(context, remote string, mode os.FileMode) error {
	local, err := ioutil.TempFile("", "go-scp-UploadFile-local")
	if err != nil {
		return errors.Wrap(err, "create tempFile")
	}
	defer os.Remove(local.Name())

	err = local.Chmod(mode)
	if err != nil {
		return errors.Wrap(err, "changes file mode")
	}

	_, err = local.WriteString(context)
	if err != nil {
		return errors.Wrap(err, "write string to file")
	}

	local.Close()

	return c.UploadFile(local.Name(), remote)
}

// Exec runs cmd on the remote host,
// returns output and error.
func (c *Client) Exec(cmd string) ([]byte, error) {
	session, err := c.c.NewSession()
	if err != nil {
		return nil, errors.Wrap(err, "ssh client new session")
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)

	return out, errors.Wrap(err, "ssh command run error")
}

// Close closes the underlying network connection
func (c *Client) Close() error {
	if c == nil || c.c == nil {
		return nil
	}

	err := c.c.Close()

	return errors.Wrap(err, "close ssh client error")
}

	`
)

func newClient() (ScpClient, error) {
	// vagrant up first

	return NewScpClient(sshAddr, username, password)
}

func TestNewClient(t *testing.T) {
	_, err := newClient()
	if err != nil {
		t.Fatal(err)
	}
}

func TestExec(t *testing.T) {
	c, err := newClient()
	if err != nil {
		t.Fatal(err, c != nil)
	}

	out, err := c.Exec("/usr/bin/whoami")
	if err != nil {
		t.Error(err, string(out))
	}

	if got, want := string(bytes.TrimSpace(out)), username; got != want {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	err = c.Close()
	if err != nil {
		t.Error(err)
	}
}

func TestUploadDir(t *testing.T) {
	c, err := newClient()
	if err != nil {
		t.Fatal(err, c != nil)
	}
	defer c.Close()

	dir, err := os.Getwd()
	if err != nil {
		t.Skip(err)
	}

	remote := "/tmp"

	err = c.UploadDir(remote, dir)
	if err != nil {
		t.Errorf("upload dir:%s to %s,%+v", dir, remote, err)
	}
}

func TestUpload(t *testing.T) {
	c, err := newClient()
	if err != nil {
		t.Fatal(err, c != nil)
	}
	defer c.Close()

	remote := "/tmp/text"

	err = c.Upload(context, remote, 0644)
	if err != nil {
		t.Errorf("upload file to %s,%+v", remote, err)
	}
}
