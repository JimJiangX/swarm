package swarm

import (
	"fmt"
	"testing"
)

func TestGo(t *testing.T) {
	ch := make(chan error, 1)

	Go(func() error {
		return fmt.Errorf("It's a error")
	}, ch)

	if err := <-ch; err == nil {
		t.Errorf("Unexpect,want error but got nil")
	}

	Go(func() error {
		return nil
	}, ch)

	if err := <-ch; err != nil {
		t.Errorf("Unexpect,want nil but got error:'%s'", err)
	}
}

func ExampleGo() {
	errFunc := func() error {
		return fmt.Errorf("It's a error")
	}
	nopFunc := func() error {
		return nil
	}

	funcs := make([]func() error, 10)
	for i := range funcs {
		if i%2 == 0 {
			funcs[i] = errFunc
		} else {
			funcs[i] = nopFunc
		}
	}

	length := len(funcs)
	ch := make(chan error, length)

	for i := range funcs {
		function := funcs[i]
		Go(function, ch)
	}

	mulErr := NewMultipleError()
	for i := 0; i < length; i++ {
		err := <-ch
		if err != nil {
			mulErr.Append(err)
		}
	}

	if err := mulErr.Err(); err != nil {
		fmt.Println(err)
	}

	// Output:
	// It's a error
	// It's a error
	// It's a error
	// It's a error
	// It's a error
}

func TestMultipleError(t *testing.T) {
	merr := NewMultipleError()

	if merr.Err() != nil {
		t.Errorf("Unexpected,got '%s'", merr.Error())
	}

	for i := 0; i < 10; i++ {
		merr.Append(nil)
	}

	if merr.Err() != nil {
		t.Errorf("Unexpected,got '%s',len=%d", merr.Error())
	}

	for i := 0; i < 10; i++ {
		merr.Append(fmt.Errorf("Error %d", i))
	}

	if merr.Err() == nil {
		t.Errorf("Unexpected,want no-nil error but got nil '%s',len=%d", merr.Error())
	}

	t.Log(merr.Error())
}
