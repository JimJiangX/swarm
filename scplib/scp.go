package scplib

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

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
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(
				PasswordKeyboardInteractive(password)),
		},
	}

	c, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, errors.Wrap(err, "new SSH client")
	}

	return &Client{c}, nil
}

// NewClientByPublicKeys returns a pointer of Client,ssh client with authenticate.
// rsa default "$HOME/.ssh/id_rsa"
func NewClientByPublicKeys(addr, user, rsa string) (*Client, error) {
	if rsa == "" {
		home := os.Getenv("HOME")
		rsa = filepath.Join(home, "/.ssh/id_rsa")
	}

	var hostKey ssh.PublicKey
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
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}

	// Connect to the remote server and perform the SSH handshake.
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		if client != nil {
			client.Close()
		}

		return nil, errors.Wrap(err, "unable to connect")
	}

	return &Client{client}, nil
}

// An implementation of ssh.KeyboardInteractiveChallenge that simply sends
// back the password for all questions. The questions are logged.
func PasswordKeyboardInteractive(password string) ssh.KeyboardInteractiveChallenge {
	return func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		//		log.Printf("Keyboard interactive challenge: ")
		//		log.Printf("-- User: %s", user)
		//		log.Printf("-- Instructions: %s", instruction)
		//		for i, question := range questions {
		//			log.Printf("-- Question %d: %s", i+1, question)
		//		}

		// Just send the password back for all questions
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = string(password)
		}

		return answers, nil
	}
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

	return c.UploadFile(remote, local.Name())
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
