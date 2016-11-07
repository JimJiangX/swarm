package scplib

import "testing"

func newClient() (*Client, error) {

	return NewClient("192.168.2.141", "root", "root")
}

func TestNewClient(t *testing.T) {
	c, err := newClient()
	if err != nil {
		t.Error(err, c != nil)
	}
}
