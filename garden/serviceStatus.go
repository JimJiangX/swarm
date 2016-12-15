package garden

import (
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

const (
	_ing = iota
	_done
	_failed
)

func isInProgress(val int) bool {
	return val&0x0F == _ing
}

func isnotInProgress(val int) bool {
	return val&0x0F != _ing
}

func isDone(val int) bool {
	return val&0x0F == _done
}

func isFailed(val int) bool {
	return val&0X0F == _failed
}

func isEqual(old int) func(val int) bool {
	return func(val int) bool {
		return old == val
	}
}

type statusLock struct {
	key      string
	retries  int
	waitTime time.Duration
	load     func(key string) (int, error)
	set      func(key string, val int, t time.Time) error
	cas      func(key string, new int, t time.Time, f func(val int) bool) (bool, int, error)
}

func (sl statusLock) Load() (int, error) {
	if sl.load == nil {
		return 0, errors.New("load is nil")
	}

	return sl.load(sl.key)
}

func (sl statusLock) CAS(val int, f func(val int) bool) (bool, int, error) {
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

		done, value, err = sl.cas(sl.key, val, time.Now(), f)
		if done {
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

func (sl statusLock) SetStatus(val int) error {
	if sl.set == nil {
		return errors.New("set is nil")
	}

	err := sl.set(sl.key, val, time.Now())
	if err == nil {
		return nil
	}

	if sl.retries < 1 {
		sl.retries = 1
	}

	t := sl.waitTime / time.Duration(sl.retries+1)

	for c := sl.retries; c > 0; c-- {
		if t > 0 {
			time.Sleep(t)
		}

		err = sl.set(sl.key, val, time.Now())
		if err == nil {
			return nil
		}

		if c == 1 {
			break
		}
	}

	return err
}

func newStatusLock(key string, ormer database.ServiceOrmer) statusLock {
	return statusLock{
		key:      key,
		retries:  3,
		waitTime: time.Second * 2,

		load: ormer.GetServiceStatus,
		set:  ormer.UpdateServiceStatus,
		cas:  ormer.ServiceStatusCAS,
	}
}
