package storage

//import (
//	"testing"
//)

//func TestIsLocalStore(t *testing.T) {
//	tests := []struct {
//		_type string
//		want  bool
//	}{
//		{"abc_local", false},
//		{"local", true},
//		{"local:HDD", true},
//		{"local:SSD", true},
//		{"local+34802jfwfhuiayf*&9", true},
//		{"Local", false},
//	}

//	for _, test := range tests {
//		got := IsLocalStore(test._type)
//		if got != test.want {
//			t.Errorf("Unexpected,%s :want %v but got %v", test._type, test.want, got)
//		}
//	}

//}
