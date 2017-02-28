package structs

import "testing"

func TestParseImage(t *testing.T) {
	v, err := ParseImage("mysql:5.7.19")
	if err != nil {
		t.Error(err, v)
	}

	if v.Name != "mysql" || v.Major != 5 || v.Minor != 7 || v.Patch != 19 {
		t.Errorf("%#v", v)
	}

	v1, err := ParseImage("mysql:5.7")
	if err != nil {
		t.Error(err, v1)
	}

	if v1.Name != "mysql" || v1.Major != 5 || v1.Minor != 7 || v1.Patch != 0 {
		t.Errorf("%#v", v1)
	}
}

func TestImageVersion(t *testing.T) {
	v := ImageVersion{"mysql", 5, 7, 19}
	if got := v.Version(); got != "mysql:5.7.19" {
		t.Error(got, "!=", "mysql:5.7.19")
	}

	v1 := ImageVersion{
		Name:  "mysql",
		Major: 4,
		Minor: 17,
	}
	if got := v1.Version(); got != "mysql:4.17.0" {
		t.Error(got, "!=", "mysql:4.17.0")
	}

	less, err := v.LessThan(v1)
	if err != nil {
		t.Error(err)
	}
	if less {
		t.Error(less, v.Version(), v1.Version())
	}

	equal := ImageVersion{
		Name:  "mysql",
		Major: 5,
		Minor: 7,
		Patch: 19,
	}

	less, err = v.LessThan(equal)
	if err != nil {
		t.Error(err)
	}
	if less {
		t.Error(less, v.Version(), equal.Version())
	}

	big := ImageVersion{
		Name:  "mysql",
		Major: 5,
		Minor: 17,
		Patch: 0,
	}

	less, err = v.LessThan(big)
	if err != nil {
		t.Error(err)
	}
	if less {
		t.Error(less, v.Version(), big.Version())
	}
}
