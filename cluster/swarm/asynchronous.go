package swarm

import (
	"bytes"

	"github.com/Sirupsen/logrus"
)

// Go is a basic promise implementation: it wraps calls a function in a goroutine,
// and returns a channel which will later return the function's return value.
func Go(f func() error, ch chan error) {
	go func() {
		ch <- f()
	}()
}

type multipleError []error

func NewMultipleError(cap int) multipleError {
	return make([]error, 0, cap)
}

func (m *multipleError) Append(err error) {
	*m = append(*m, err)
}

func (m multipleError) Error() string {
	if len(m) == 0 {
		return ""
	}
	buffer := bytes.NewBuffer(nil)

	for i := range m {
		if m[i] != nil {
			_, err := buffer.WriteString(m[i].Error())
			if err != nil {
				logrus.Errorf("WriteString '%s' error:%s", m[i], err)
			}
			buffer.WriteString("\n")
		}
	}

	return buffer.String()
}

func (m multipleError) Err() error {
	if len(m) == 0 {
		return nil
	}

	all := true

	for i := range m {
		if m[i] != nil {
			all = false
			break
		}
	}

	if all {
		return nil
	}

	return m
}
