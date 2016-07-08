package swarm

import (
	"bytes"
	"time"

	"github.com/Sirupsen/logrus"
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

type taskRecorder interface {
	Insert() error
	Update(code int, msg string) error
}

type asyncTask struct {
	recorder   taskRecorder
	timeout    time.Duration
	background taskFunc
	parent     context.Context
	cancel     func()
}

type taskFunc func(context.Context) error

func NewAsyncTask(ctx context.Context, f taskFunc, recorder taskRecorder, timeout time.Duration) *asyncTask {
	return &asyncTask{
		recorder:   recorder,
		timeout:    timeout,
		background: f,
		parent:     ctx,
	}
}

func (t *asyncTask) Run() error {
	err := t.recorder.Insert()
	if err != nil {
		return err
	}

	if t.parent == nil {
		t.parent = context.Background()
	}

	ctx, cancel := context.WithTimeout(t.parent, t.timeout)
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

		if code != 0 {
			err = t.recorder.Update(code, msg)
			if err != nil {
				logrus.Errorf("taskRecorder Update Error:%s,Code=%d,message=%s", err, code, msg)
			}
		}
	}(ctx, t)

	return nil
}
