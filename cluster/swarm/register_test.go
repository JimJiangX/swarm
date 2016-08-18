package swarm

import "testing"

func TestDeregisterToHorus(t *testing.T) {

	body := []string{"1234567890", "0987654321"}

	err := deregisterToHorus(true, body...)
	if err != nil {
		t.Error(err)
	}
}
