package scplib

import (
	"bytes"
	"testing"
)

const (
	sshAddr  = "127.0.0.1:2222"
	username = "vagrant"
	password = "vagrant"
)

func newClient() (*Client, error) {
	// vagrant up first

	return NewClient(sshAddr, username, password)
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
