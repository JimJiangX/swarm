package tasklock

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

type goTaskLock struct {
	current int
	expect  int
	fail    int

	task          database.Task
	key           string
	retries       int
	waitTime      time.Duration
	load          func(key string) (int, error)
	set           func(key string, val int, task database.Task, t time.Time) error
	casInsertTask func(key string, new int, t time.Time, task database.Task, f func(val int) bool) (bool, int, error)
}

func (tl goTaskLock) Load() (int, error) {
	if tl.load == nil {
		return 0, errors.New("load is nil")
	}

	return tl.load(tl.key)
}

func (tl goTaskLock) _CAS(f func(val int) bool) (bool, int, error) {
	if tl.casInsertTask == nil || f == nil {
		return false, 0, errors.New("cas or f is nil")
	}

	if tl.retries <= 0 {
		tl.retries = 1
	}

	var (
		done  bool
		err   error
		value int
		now   = time.Now()
		t     = tl.waitTime / time.Duration(tl.retries)
	)

	for c := tl.retries; c > 0; c-- {

		done, value, err = tl.casInsertTask(tl.key, tl.current, now, tl.task, f)
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

func (tl goTaskLock) setStatus(val int) error {
	if tl.set == nil {
		return errors.New("set is nil")
	}

	now := time.Now()
	err := tl.set(tl.key, val, tl.task, now)
	if err == nil {
		return nil
	}

	if tl.retries < 1 {
		tl.retries = 1
	}

	t := tl.waitTime / time.Duration(tl.retries+1)

	for c := tl.retries; c > 0; c-- {
		if t > 0 {
			time.Sleep(t)
		}

		err = tl.set(tl.key, val, tl.task, now)
		if err == nil {
			return nil
		}

		if c == 1 {
			break
		}
	}

	return err
}

func (tl goTaskLock) Run(before func(val int) bool, do func() error) error {
	go func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf("panic:%v", r)
			}

			val := tl.expect
			tl.task.Status = database.TaskDoneStatus
			tl.task.Errors = ""
			if err != nil {
				val = tl.fail
				tl.task.Status = database.TaskFailedStatus
				tl.task.Errors = err.Error()

				logrus.WithField("key", tl.key).Errorf("go task lock:%+v", err)
			}

			_err := tl.setStatus(val)
			if _err != nil {
				logrus.WithField("key", tl.key).Errorf("go task lock:setStatus error,%+v", _err)
			}

		}()

		return do()
	}()

	return nil
}

func NewServiceTask(key string, ormer database.ServiceOrmer,
	t database.Task, current, expect, fail int) goTaskLock {

	return goTaskLock{
		current: current,
		expect:  expect,
		fail:    fail,

		task:     t,
		key:      key,
		retries:  3,
		waitTime: time.Second * 2,

		//		load:          ormer.GetServiceStatus,
		//		set:           ormer.SetServiceStatus,
		//		casInsertTask: ormer.ServiceStatusCAS,
	}
}

type statusError struct {
	got    int
	expect int
}

func (se statusError) Error() string {
	return fmt.Sprintf("expected %d bug got %d", se.expect, se.got)
}

func newStatusError(expect, got int) error {
	return statusError{
		got:    got,
		expect: expect,
	}
}
