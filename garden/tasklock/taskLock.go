package tasklock

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

// GoTaskLock is a workflow do something synchronous or asynchronous
type GoTaskLock struct {
	current int
	expect  int
	fail    int

	task     *database.Task
	key      string
	retries  int
	waitTime time.Duration
	load     func(key string) (int, error)
	After    func(key string, val int, task *database.Task, t time.Time) error
	Before   func(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error)
}

// Load load current value by key
func (tl GoTaskLock) Load() (int, error) {
	if tl.load == nil {
		return 0, errors.New("load is nil")
	}

	return tl.load(tl.key)
}

func (tl GoTaskLock) _CAS(f func(val int) bool) (bool, int, error) {
	if tl.Before == nil || f == nil {
		return false, 0, errors.New("cas or f is nil")
	}

	if tl.retries <= 0 {
		tl.retries = 1
	}

	var (
		done  bool
		err   error
		value int
		t     = tl.waitTime / time.Duration(tl.retries)
	)

	for c := tl.retries; c > 0; c-- {

		done, value, err = tl.Before(tl.key, tl.current, tl.task, f)
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

func (tl GoTaskLock) setStatus(val int) error {
	if tl.After == nil {
		return errors.New("set is nil")
	}

	now := time.Now()
	err := tl.After(tl.key, val, tl.task, now)
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

		err = tl.After(tl.key, val, tl.task, now)
		if err == nil {
			return nil
		}

		if c == 1 {
			break
		}
	}

	return err
}

// Go runs do func asynchronous
func (tl GoTaskLock) Go(check func(val int) bool, do func() error) error {
	return tl.run(check, do, true)
}

// Run runs do func synchronous
func (tl GoTaskLock) Run(check func(val int) bool, do func() error, async bool) error {
	return tl.run(check, do, async)
}

func (tl GoTaskLock) run(check func(val int) bool, do func() error, async bool) error {
	done, val, err := tl._CAS(check)
	if err != nil {
		return err
	}
	if !done {
		return errors.WithStack(newStatusError(tl.current, val))
	}

	action := func() (err error) {
		start := time.Now()
		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf("panic:%v", r)
			}

			field := logrus.WithFields(logrus.Fields{
				"Key": tl.key,
			})

			if tl.task != nil {
				tl.task.Status = database.TaskDoneStatus
				tl.task.SetErrors(err)
				tl.task.FinishedAt = time.Now()

				field.WithField("Name", tl.task.Name)

				if err != nil {
					tl.task.Status = database.TaskFailedStatus
				}
			}

			val := tl.expect
			if err != nil {
				val = tl.fail
			}

			_err := tl.setStatus(val)
			if _err != nil {
				field.Errorf("go task lock:setStatus error,%+v", _err)
			}

			field.Infof("Task Done! Since=%s %+v", time.Since(start), err)
		}()

		return do()
	}

	if !async {
		return action()
	}

	go action()

	return nil
}

// NewGoTask returns GoTaskLock
func NewGoTask(key string, task *database.Task,
	before func(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error),
	after func(key string, val int, task *database.Task, t time.Time) error) GoTaskLock {

	return GoTaskLock{
		task:     task,
		key:      key,
		retries:  3,
		waitTime: time.Second * 2,
		After:    after,
		Before:   before,
	}
}

// NewServiceTask returns a GoTaskLock,init by ServiceOrmer
func NewServiceTask(key string, ormer database.ServiceOrmer,
	t *database.Task, current, expect, fail int) GoTaskLock {

	return GoTaskLock{
		current: current,
		expect:  expect,
		fail:    fail,

		task:     t,
		key:      key,
		retries:  3,
		waitTime: time.Second * 2,

		load:   ormer.GetServiceStatus,
		After:  ormer.SetServiceWithTask,
		Before: ormer.ServiceStatusCAS,
	}
}

type statusError struct {
	got    int
	expect int
}

func (se statusError) Error() string {
	return fmt.Sprintf("expected %d but got %d", se.expect, se.got)
}

func newStatusError(expect, got int) error {
	return statusError{
		got:    got,
		expect: expect,
	}
}
