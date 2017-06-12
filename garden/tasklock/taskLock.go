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

	task     *database.Task
	key      string
	retries  int
	waitTime time.Duration
	load     func(key string) (int, error)
	after    func(key string, val int, task *database.Task, t time.Time) error
	before   func(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error)
}

func (tl goTaskLock) Load() (int, error) {
	if tl.load == nil {
		return 0, errors.New("load is nil")
	}

	return tl.load(tl.key)
}

func (tl goTaskLock) _CAS(f func(val int) bool) (bool, int, error) {
	if tl.before == nil || f == nil {
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

		done, value, err = tl.before(tl.key, tl.current, tl.task, f)
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
	if tl.after == nil {
		return errors.New("set is nil")
	}

	now := time.Now()
	err := tl.after(tl.key, val, tl.task, now)
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

		err = tl.after(tl.key, val, tl.task, now)
		if err == nil {
			return nil
		}

		if c == 1 {
			break
		}
	}

	return err
}

func (tl goTaskLock) Go(check func(val int) bool, do func() error) error {
	return tl.run(check, do, true)
}

func (tl goTaskLock) Run(check func(val int) bool, do func() error, async bool) error {
	return tl.run(check, do, async)
}

func (tl goTaskLock) run(check func(val int) bool, do func() error, async bool) error {
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
			now := time.Now()

			field := logrus.WithFields(logrus.Fields{
				"Key":   tl.key,
				"Since": now.Sub(start).String(),
			})

			val := tl.expect

			if tl.task != nil {
				tl.task.Status = database.TaskDoneStatus
				tl.task.SetErrors(err)
				tl.task.FinishedAt = now
			}

			if err != nil {
				val = tl.fail

				if tl.task != nil {
					tl.task.Status = database.TaskFailedStatus
				}

				field.Errorf("go task lock:%+v", err)
			}

			_err := tl.setStatus(val)
			if _err != nil {
				field.Errorf("go task lock:setStatus error,%+v", _err)
			}

			field.Debug("Task Done!")
		}()

		return do()
	}

	if !async {
		return action()
	}

	go action()

	return nil
}

func (tl *goTaskLock) SetBefore(before func(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error)) {
	tl.before = before
}

func (tl *goTaskLock) SetAfter(after func(key string, val int, task *database.Task, t time.Time) error) {
	tl.after = after
}

// NewGoTask returns goTaskLock
func NewGoTask(key string, task *database.Task,
	before func(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error),
	after func(key string, val int, task *database.Task, t time.Time) error) goTaskLock {

	return goTaskLock{
		task:     task,
		key:      key,
		retries:  3,
		waitTime: time.Second * 2,
		after:    after,
		before:   before,
	}
}

// NewServiceTask returns a goTaskLock,init by ServiceOrmer
func NewServiceTask(key string, ormer database.ServiceOrmer,
	t *database.Task, current, expect, fail int) goTaskLock {

	return goTaskLock{
		current: current,
		expect:  expect,
		fail:    fail,

		task:     t,
		key:      key,
		retries:  3,
		waitTime: time.Second * 2,

		load:   ormer.GetServiceStatus,
		after:  ormer.SetServiceWithTask,
		before: ormer.ServiceStatusCAS,
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
