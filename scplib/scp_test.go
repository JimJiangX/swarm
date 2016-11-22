package scplib

import "testing"

func newClient() (*Client, error) {

	return NewClient("127.0.0.1", "fugr", "No.001")
}

func TestNewClient(t *testing.T) {
	c, err := newClient()
	if err != nil {
		t.Error(err, c != nil)
	}

	err = c.Close()
	if err != nil {
		t.Error(err)
	}
}
