package scplib

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"time"

	scp "github.com/hnakamur/go-scp"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// ScpClient is exported
type ScpClient interface {
	// copy context to remote file.
	Upload(context, remote string, mode os.FileMode) error

	// UploadFile copy local file to remote
	UploadFile(remote, local string) error

	// UploadDir copy dir files to remote
	UploadDir(remote, local string) error

	// Exec exec command
	Exec(cmd string) ([]byte, error)

	// Close close ScpClient
	Close() error
}

var _SSHPorts = []string{"22", "2222"}

// client contains SSH client.
type client struct {
	c *ssh.Client
}

// NewScpClient returns ScpClient.
func NewScpClient(addr, user, password string, timeout time.Duration) (ScpClient, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(
				passwordKeyboardInteractive(password)),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	c, err := makeSSHClient(addr, config)
	if err != nil {
		return nil, err
	}

	return &client{c}, nil
}

func makeSSHClient(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	_, _, err := net.SplitHostPort(addr)
	if err == nil {
		// Connect to the remote server and perform the SSH handshake.
		c, err := ssh.Dial("tcp", addr, config)
		if err == nil && c != nil {
			return c, nil
		}

		if c != nil {
			c.Close()
		}

		return nil, errors.Wrap(err, "new SSH client")
	}

	if net.ParseIP(addr) == nil {
		return nil, errors.Wrap(err, "parse addr error:"+addr)
	}

	for i := range _SSHPorts {
		addr = net.JoinHostPort(addr, _SSHPorts[i])
		// Connect to the remote server and perform the SSH handshake.
		c, err := ssh.Dial("tcp", addr, config)
		if err == nil && c != nil {
			return c, nil
		}

		if c != nil {
			c.Close()
		}
	}

	return nil, errors.Wrap(err, "new SSH client")
}

// NewClientByPublicKeys returns ScpClient,ssh client with authenticate.
// rsa default "$HOME/.ssh/id_rsa"
func NewClientByPublicKeys(addr, user, rsa string, timeout time.Duration) (ScpClient, error) {
	if rsa == "" {
		home := os.Getenv("HOME")
		rsa = filepath.Join(home, "/.ssh/id_rsa")
	}

	// A public key may be used to authenticate against the remote
	// server by using an unencrypted PEM-encoded private key file.
	//
	// If you have an encrypted private key, the crypto/x509 package
	// can be used to decrypt it.
	key, err := ioutil.ReadFile(rsa)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read private key")
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse private key")
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			// Use the PublicKeys method for remote authentication.
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	c, err := makeSSHClient(addr, config)
	if err != nil {
		return nil, err
	}

	return &client{c}, nil
}

// An implementation of ssh.KeyboardInteractiveChallenge that simply sends
// back the password for all questions. The questions are logged.
func passwordKeyboardInteractive(password string) ssh.KeyboardInteractiveChallenge {
	return func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		// Just send the password back for all questions
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = password
		}

		return answers, nil
	}
}

// UploadDir copies files and directories under the local dir
// to the remote dir.
func (c *client) UploadDir(remote, local string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendDir(local, remote, nil)

	return errors.Wrap(err, "upload dir")
}

// UploadFile copies a single local file to the remote server.
func (c *client) UploadFile(remote, local string) error {
	cli := scp.NewSCP(c.c)

	err := cli.SendFile(local, remote)

	return errors.Wrap(err, "upload file")
}

// Upload upload string to the remote server.
func (c *client) Upload(context, remote string, mode os.FileMode) error {
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

	return c.UploadFile(remote, local.Name())
}

// Exec runs cmd on the remote host,
// returns output and error.
func (c *client) Exec(cmd string) ([]byte, error) {
	session, err := c.c.NewSession()
	if err != nil {
		return nil, errors.Wrap(err, "ssh client new session")
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	if err == nil {
		return out, err
	}

	return out, errors.Wrap(err, "ssh command run error")
}

// Close closes the underlying network connection
func (c *client) Close() error {
	if c == nil || c.c == nil {
		return nil
	}

	err := c.c.Close()

	return errors.Wrap(err, "close ssh client error")
}
