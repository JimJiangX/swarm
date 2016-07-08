package swarm

import "bytes"

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
