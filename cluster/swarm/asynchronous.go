package swarm

import (
	"bytes"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Go is a basic promise implementation: it wraps calls a function in a goroutine,
// and returns a channel which will later return the function's return value.
func Go(f func() error, ch chan error) {
	go func() {
		ch <- f()
	}()
}

type multipleError struct {
	buffer *bytes.Buffer
	val    string
}

func NewMultipleError() multipleError {
	return multipleError{
		buffer: bytes.NewBuffer(nil),
	}
}

func (m *multipleError) Append(err error) {
	if err == nil {
		return
	}
	if m.buffer == nil {
		m.buffer = bytes.NewBuffer(nil)
	}
	m.buffer.WriteString(err.Error())
	m.buffer.WriteString("\n")

	m.val = m.buffer.String()
}

func (m multipleError) Error() string {
	return m.val
}

func (m multipleError) Err() error {
	if m.buffer == nil {
		return nil
	}
	if m.buffer.Len() == 0 {
		return nil
	}

	return m
}

func GoConcurrency(funcs ...func() error) error {
	length := len(funcs)
	ch := make(chan error, length)

	for i := range funcs {
		go func(f func() error) {
			ch <- f()
		}(funcs[i])
	}

	errs := NewErrors()
	for i := 0; i < length; i++ {
		errs.Append(<-ch)
	}

	return errs
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

	select {
	case <-t.parent.Done():
		return errors.Wrap(t.parent.Err(), "parent Context has done")
	default:
	}

	if t.create != nil {
		if err := t.create(); err != nil {
			return errors.Wrap(err, "create error")
		}
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
		defer t.cancel()

		msg, code := "", 0

		logrus.Debug("Running background...")

		err := t.background(ctx)
		if err == nil {
			code = _StatusTaskDone
		} else {
			code = _StatusTaskFailed
			msg = err.Error()
		}

		logrus.Info("Run background end,", msg)

		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				if err == context.DeadlineExceeded {
					msg = "Timeout " + msg
					code = _StatusTaskTimeout
				} else if err == context.Canceled {
					msg = "Canceled " + msg
					code = _StatusTaskCancel
				}
			}
		default:
		}

		if code != 0 && t.update != nil {
			err = t.update(code, msg)
			if err != nil {
				logrus.Errorf("taskRecorder Update Error:%s,Code=%d,message=%s", err, code, msg)
			}
		}
	}(ctx, t)

	return nil
}
