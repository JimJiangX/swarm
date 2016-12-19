package resource

import "testing"

func TestReduceCPUset(t *testing.T) {
	want := "0,2,3,4,5"
	got, err := reduceCPUset("11,2,6,9,3,4,5,0", 5)
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

func TestFindIdleCPUs(t *testing.T) {
	want := "2,4,6,7"
	got, err := findIdleCPUs([]string{"0,1", "5,8", "3", "9"}, 10, 4)
	if err != nil {
		t.Error(err)
	}
	if got != want {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	want = "2,4,6,10"
	got, err = findIdleCPUs([]string{"0,1", "5,8", "3", "7,9"}, 11, 4)
	if err != nil {
		t.Error(err)
	}
	if got != want {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	got, err = findIdleCPUs([]string{"0,1", "5,8", "3", "7,9"}, 11, 5)
	if err == nil {
		t.Errorf("error expected,but got '%s'", got)
	}
	t.Log(got, err)
}
