package swarm

import (
	"sync"
	"time"

	"github.com/pkg/errors"
)

func IsStatusInProgress(val int) bool {
	return val&0x0F == _ing
}

func IsStatusDone(val int) bool {
	return val&0x0F == _done
}

func IsStatusFailure(val int) bool {
	return val&0X0F == _failed
}

func IsStatusEqual(old int) func(val int) bool {
	return func(val int) bool {
		return old == val
	}
}

type statusLock struct {
	key      string
	retries  int
	waitTime time.Duration
	lock     *sync.Mutex
	load     func(key string) (int, error)
	set      func(key string, val int) error
	cas      func(key string, new int, f func(val int) bool) (bool, int, error)
}

func (sl statusLock) Load() (int, error) {
	if sl.load == nil {
		return 0, errors.New("load is nil")
	}

	return sl.load(sl.key)
}

func (sl statusLock) CasAndLock(val int, f func(val int) bool) (bool, int, error) {
	if sl.cas == nil || f == nil {
		return false, 0, errors.New("cas or f is nil")
	}

	if sl.retries <= 0 {
		sl.retries = 1
	}

	var (
		done  bool
		err   error
		value int
		t     = sl.waitTime / time.Duration(sl.retries)
	)

	for c := sl.retries; c > 0; c-- {

		done, value, err = sl.cas(sl.key, val, f)
		if done {
			sl.lock.Lock()
			return done, value, err
		}

		if c == 1 {
			break
		}

		if t > 0 {
			time.Sleep(t)
		}
	}

	return done, value, err
}

func (sl statusLock) ReleaseLock(val int) error {
	if sl.set == nil {
		return errors.New("set is nil")
	}

	if sl.retries <= 0 {
		sl.retries = 1
	}

	var (
		err error
		t   = sl.waitTime / time.Duration(sl.retries)
	)

	for c := sl.retries; c > 0; c-- {

		err = sl.set(sl.key, val)
		if err == nil {
			sl.lock.Unlock()
			return nil
		}

		if c == 1 {
			break
		}

		if t > 0 {
			time.Sleep(t)
		}
	}

	return err
}
