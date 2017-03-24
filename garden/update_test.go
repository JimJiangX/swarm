package garden

import "testing"

func TestReduceCPUset(t *testing.T) {
	want := "0,2,3,4,5"
	got, err := reduceCPUset(",11,2,6,9,3,4,5,0", 5)
	if err != nil {
		t.Error(err)
	}
	if want != got {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	want = "0,1,2,3,4,5,6,9"
	got, err = reduceCPUset("1,1,2,6,9,3,4,5,0,9,3,4,5,0", 8)
	if err != nil {
		t.Error(err)
	}
	if want != got {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	got, err = reduceCPUset("1,1,2,6,9,3,4,5,0,9,3,4,5,0", 9)
	if err == nil {
		t.Error("error expected")
	}

	t.Log(got, err)
}
