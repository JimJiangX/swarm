package database

import "testing"

func TestCombin(t *testing.T) {
	tests := []NetworkingRequire{
		{Networking: "aaaaa", Bandwidth: 1},
	}

	add := []NetworkingRequire{
		{Networking: "bbbbb", Bandwidth: 2},
		{Networking: "ccccc", Bandwidth: 3},
		{Networking: "bbbbb", Bandwidth: 4},
		{Networking: "aaaaa", Bandwidth: 5},
		{Networking: "aaaaa", Bandwidth: 6},
	}

	out := combin(nil)
	if out != nil {
		t.Errorf("Unexpected,len(out)=%d", len(out))
	}

	out = combin(tests)
	if len(out) != 1 {
		t.Errorf("Unexpected,len(out)=%d", len(out))
	}

	t.Log(out)

	tests = append(tests, add[0:2]...)
	out = combin(tests)
	if len(out) != 3 {
		t.Errorf("Unexpected,len(tests)=%d len(out)=%d", len(tests), len(out))
	}

	t.Log(out)

	tests = append(tests, add[2:]...)
	out = combin(tests)
	if len(out) != 3 {
		t.Errorf("Unexpected,len(tests)=%d len(out)=%d", len(tests), len(out))
	}

	t.Log(out)
}
