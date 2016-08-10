package swarm

import "testing"

func TestDeregisterToHorus(t *testing.T) {
	addr := "192.168.2.123:8000"
	body := []string{"1234567890", "0987654321"}

	err := deregisterToHorus(addr, true, body...)
	if err != nil {
		t.Error(err)
	}
}
