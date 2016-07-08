package swarm

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"
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
		t.Errorf("Unexpected,want nil error,got '%s'", merr.Error())
	}

	for i := 0; i < 10; i++ {
		merr.Append(nil)
	}

	if merr.Err() != nil {
		t.Errorf("Unexpected,want nil error,got '%s'", merr.Error())
	}

	for i := 0; i < 10; i++ {
		merr.Append(fmt.Errorf("Error %d", i))
	}

	if merr.Err() == nil {
		t.Errorf("Unexpected,want no-nil error but got nil '%s'", merr.Error())
	}

	t.Log(merr.Error())
}

type recorder struct{}

func (recorder) Insert() error {
	fmt.Println("call recorder.Insert")

	return nil
}

func (*recorder) Update(code int, msg string) error {
	fmt.Println("call recorder.Update")

	return nil
}

type recorder1 struct{}

func (recorder1) Insert() error {
	fmt.Println("call recorder1.Insert")

	return nil
}

func (*recorder1) Update(code int, msg string) error {

	return fmt.Errorf("call recorder1.Update error")
}

type recorder2 struct{}

func (recorder2) Insert() error {

	return fmt.Errorf("call recorder2.Insert")
}

func (*recorder2) Update(code int, msg string) error {

	return fmt.Errorf("call recorder2.Update error")
}

func ExampleAsyncTask() {
	recorders := []taskRecorder{
		&recorder{},
		&recorder1{},
		&recorder2{},
	}

	tasks := []taskFunc{
		func(ctx context.Context) (err error) {
			select {
			case <-ctx.Done():
				err = ctx.Err()
			default:
			}
			return err
		},
		func(ctx context.Context) (err error) {
			time.Sleep(time.Second * 2)
			select {
			case <-ctx.Done():
				err = ctx.Err()
			default:
			}
			return err
		},
		func(ctx context.Context) (err error) {
			time.Sleep(time.Second * 10)
			select {
			case <-ctx.Done():
				err = ctx.Err()
			default:
			}
			deadline, ok := ctx.Deadline()
			return fmt.Errorf("It's a timeout error %v,deadline=%s,%t", err, deadline, ok)
		},
	}

	for i := range tasks {
		for j := range recorders {
			ctx := context.Background()

			t := NewAsyncTask(ctx, tasks[i], recorders[j], time.Second*5)
			t.Run()

			if i == 1 && t.cancel != nil {
				t.cancel()
			}
		}
	}

	time.Sleep(time.Second * 20)

	// output:
	// call recorder.Insert
	// call recorder1.Insert
	// call recorder.Insert
	// call recorder1.Insert
	// call recorder.Insert
	// call recorder1.Insert
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
}
