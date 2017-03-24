package resource

import "testing"

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
