package swarm

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"
)

func TestGoConcurrency(t *testing.T) {
	errFunc := func() error {
		return fmt.Errorf("It's a error")
	}
	nopFunc := func() error {
		return nil
	}

	err := GoConcurrency([]func() error{errFunc})
	if err == nil {
		t.Errorf("Unexpect,want error but got nil")
	}

	err = GoConcurrency([]func() error{nopFunc})
	if err != nil {
		t.Errorf("Unexpect,want nil but got error:'%s'", err)
	}

	funcs := make([]func() error, 10)
	for i := range funcs {
		if i%2 == 0 {
			funcs[i] = errFunc
		} else {
			funcs[i] = nopFunc
		}
	}

	err = GoConcurrency(funcs)
	if err == nil {
		t.Errorf("Unexpect,want error but got nil")
	}

	if _errs, ok := err.(_errors); !ok {
		t.Error("Unexpected,%s", err)
	} else if errs := _errs.Split(); len(errs) != 5 {
		t.Error("Unexpected,%s", errs)
	}

	t.Logf("%s", err)
}

func TestErrors(t *testing.T) {
	errs := NewErrors()

	if errs.Err() != nil {
		t.Errorf("Unexpected,want nil error,got '%s'", errs.Error())
	}

	for i := 0; i < 10; i++ {
		errs.Append(nil)
	}

	if errs.Err() != nil {
		t.Errorf("Unexpected,want nil error,got '%s'", errs.Error())
	}

	for i := 0; i < 25; i++ {
		errs.Append(fmt.Errorf("Error %d", i))
	}

	if errs.Err() == nil {
		t.Errorf("Unexpected,want no-nil error but got nil '%s'", errs.Error())
	}

	_errs := errs.Split()
	if len(_errs) != 25 {
		t.Errorf("Unexpected,want 25 but got %d", len(_errs))
	}

	for i := range _errs {
		t.Log(i, _errs[i])
	}
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
	var (
		r0 = &recorder{}
		r1 = &recorder1{}
		r2 = &recorder2{}
	)

	creates := []func() error{
		r0.Insert,
		r1.Insert,
		r2.Insert,
	}

	updates := []func(code int, msg string) error{
		r0.Update,
		r1.Update,
		r2.Update,
	}

	tasks := []func(context.Context) error{
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
		for c := range creates {
			for u := range updates {
				ctx := context.Background()

				t := NewAsyncTask(ctx, tasks[i], creates[c], updates[u], time.Second*5)
				err := t.Run()
				if err != nil {
					fmt.Println(err)
				}

				if i == 1 && t.cancel != nil {
					t.cancel()
				}
			}
		}
	}

	time.Sleep(time.Second * 20)

	// output:
	// call recorder.Insert
	// call recorder.Insert
	// call recorder.Insert
	// call recorder1.Insert
	// call recorder1.Insert
	// call recorder1.Insert
	// create error: call recorder2.Insert
	// create error: call recorder2.Insert
	// create error: call recorder2.Insert
	// call recorder.Insert
	// call recorder.Insert
	// call recorder.Insert
	// call recorder1.Insert
	// call recorder1.Insert
	// call recorder1.Insert
	// create error: call recorder2.Insert
	// create error: call recorder2.Insert
	// create error: call recorder2.Insert
	// call recorder.Insert
	// call recorder.Insert
	// call recorder.Insert
	// call recorder1.Insert
	// call recorder1.Insert
	// call recorder1.Insert
	// create error: call recorder2.Insert
	// create error: call recorder2.Insert
	// create error: call recorder2.Insert
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
	// call recorder.Update
}
