package swarm

import (
	"bytes"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func GoConcurrency(funcs []func() error) error {
	errs := NewErrors()
	if len(funcs) == 0 {
		return nil
	} else if len(funcs) == 1 {
		errs.Append(funcs[0]())

		return errs.Err()
	}

	length := len(funcs)
	ch := make(chan error, length)

	for i := range funcs {
		go func(f func() error) {
			if f == nil {
				ch <- nil
			}

			defer func() {
				if r := recover(); r != nil {
					logrus.Errorf("GoConcurrency Panic:%v\n%s", r, debug.Stack())
				}
			}()

			ch <- f()

		}(funcs[i])
	}

	for i := 0; i < length; i++ {
		errs.Append(<-ch)
	}

	return errs.Err()
}

type _errors struct {
	buffer *bytes.Buffer
	val    string
	errors []error
}

func NewErrors() _errors {
	return _errors{
		buffer: bytes.NewBuffer(nil),
		errors: make([]error, 0, 10),
	}
}

func (es *_errors) Append(err error) {
	if err == nil {
		return
	}
	if es.buffer == nil {
		es.buffer = bytes.NewBuffer(nil)
	}
	es.buffer.WriteString(err.Error())
	es.buffer.WriteString("\n")

	es.val = es.buffer.String()

	if es.errors == nil {
		es.errors = make([]error, 0, 10)
	}
	es.errors = append(es.errors, err)
}

func (es _errors) Error() string {
	return es.val
}

func (es _errors) Err() error {
	if len(es.errors) == 0 ||
		es.buffer == nil ||
		es.buffer.Len() == 0 {
		return nil
	}

	return es
}

func (es _errors) Split() []error {
	if len(es.errors) == 0 {
		return nil
	}

	return es.errors
}

type asyncTask struct {
	timeout    time.Duration
	parent     context.Context
	cancel     func()
	create     func() error
	background func(context.Context) error
	update     func(code int, msg string) error
}

func NewAsyncTask(ctx context.Context,
	background func(context.Context) error,
	create func() error,
	update func(code int, msg string) error,
	timeout time.Duration) *asyncTask {

	return &asyncTask{
		timeout:    timeout,
		parent:     ctx,
		create:     create,
		background: background,
		update:     update,
	}
}

func (t *asyncTask) Run() error {
	if t.background == nil {
		return errors.New("background function is nil")
	}
	if t.parent == nil {
		t.parent = context.Background()
	}

	if t.create != nil {
		if err := t.create(); err != nil {
			return errors.Wrap(err, "create error")
		}
	}

	select {
	case <-t.parent.Done():

		if t.update != nil {
			code, msg := statusTaskCancel, "Task Cancel by Parent Context"
			if t.parent.Err() != nil {
				msg = t.parent.Err().Error()
			}

			err := t.update(code, msg)
			if err != nil {
				err = errors.Errorf("asyncTask.update,code=%d,msg=%s,%s", code, msg, err)
			}

			return errors.Wrap(err, "parent Context has done")
		}

		return errors.Wrap(t.parent.Err(), "parent Context has done")
	default:
	}

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if t.timeout == 0 {
		ctx, cancel = context.WithCancel(t.parent)
	} else {
		ctx, cancel = context.WithTimeout(t.parent, t.timeout)
	}
	t.cancel = cancel

	go func(ctx context.Context, t *asyncTask) {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("asyncTask panic:%v\n%s", r, debug.Stack())

				if t.update != nil {
					code, msg := statusTaskFailed, fmt.Sprintf("panic:%v", r)
					err := t.update(code, msg)
					if err != nil {
						logrus.Errorf("Update Error:%s,Code=%d,message=%s", err, code, msg)
					}
				}
			}
			t.cancel()
		}()

		msg, code := "", statusTaskRunning
		if t.update != nil {
			if err := t.update(code, msg); err != nil {
				logrus.Errorf("asyncTask.update error:%s", err)
			}
		}

		logrus.Debug("Running background...")

		err := t.background(ctx)
		if err == nil {
			code = statusTaskDone
		} else {
			code = statusTaskFailed
			msg = err.Error()
		}

		logrus.Info("Run background end,", msg)

		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				if err == context.DeadlineExceeded {
					msg = "Timeout " + msg
					code = statusTaskTimeout
				} else if err == context.Canceled {
					msg = "Canceled " + msg
					code = statusTaskCancel
				}
			}
		default:
		}

		if code != 0 && t.update != nil {
			err = t.update(code, msg)
			if err != nil {
				logrus.Errorf("Update Error:%s,Code=%d,message=%s", err, code, msg)
			}
		}
	}(ctx, t)

	return nil
}
