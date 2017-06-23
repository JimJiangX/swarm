package tasklock

import (
	"errors"
	"testing"
	"time"

	"github.com/docker/swarm/garden/database"
)

type statusM struct {
	m    map[string]int
	task *database.Task
}

func (s statusM) load(key string) (int, error) {

	return s.m[key], nil
}

func (s *statusM) after(key string, val int, task *database.Task, t time.Time) error {
	s.m[key] = val
	s.task = task

	return nil
}

func (s *statusM) before(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error) {
	ok := f(s.m[key])
	if ok {
		s.m[key] = new
		s.task = t
	}

	return ok, s.m[key], nil
}

func newStatusM() *statusM {
	return &statusM{
		m: make(map[string]int),
	}
}

func newStatusLock(key string, t *database.Task, origin, current, expect, fail int) GoTaskLock {
	sm := &statusM{
		m: make(map[string]int),
	}
	sm.m[key] = origin

	return GoTaskLock{
		current: current,
		expect:  expect,
		fail:    fail,

		task:     t,
		key:      key,
		retries:  3,
		waitTime: time.Second * 2,

		load:   sm.load,
		After:  sm.after,
		Before: sm.before,
	}
}

func TestTasklock(t *testing.T) {
	key := "key0001"

	// sync,nil task,goes fine
	sl0 := newStatusLock(key, nil, 0, 1, 2, 3)
	err := sl0.Run(func(val int) bool {
		return val == 0
	}, func() error {
		return nil
	}, false)
	if err != nil {
		t.Error(err)
	}

	if v, err := sl0.load(key); err != nil || v != 2 {
		t.Error(err, v)
	}

	// sync,non-nil task,goes fine
	task := &database.Task{}
	sl1 := newStatusLock(key, task, 0, 1, 2, 3)
	err = sl1.Run(func(val int) bool {
		return val == 0
	}, func() error {
		return nil
	}, false)
	if err != nil {
		t.Error(err)
	}

	if v, err := sl1.load(key); err != nil || v != 2 {
		t.Error(err, v)
	}

	if task.Status != database.TaskDoneStatus {
		t.Error(task.Status)
	} else {
		t.Logf("%+v", task)
	}

	// sync,non-nil task,befer check error
	task = &database.Task{}
	sl2 := newStatusLock(key, task, 0, 1, 2, 3)
	err = sl2.Run(func(val int) bool {
		return val == 1
	}, func() error {
		return nil
	}, false)
	if err == nil {
		t.Error("expect error")
	} else {
		t.Log(err)
	}

	if task.Status != 0 {
		t.Errorf("%+v", task)
	}

	// sync,non-nil task,doSomething() return error
	task = &database.Task{}
	sl3 := newStatusLock(key, task, 0, 1, 2, 3)
	err = sl3.Run(func(val int) bool {
		return val == 0
	}, func() error {
		return errors.New("error expected")
	}, false)
	if err == nil {
		t.Error("expect error")
	} else {
		t.Log(err)
	}

	if v, err := sl3.load(key); err != nil || v != 3 {
		t.Error(err, v)
	}

	if task.Status != database.TaskFailedStatus {
		t.Error(task.Status)
	} else {
		t.Logf("%+v", task)
	}

	// async==true,non-nil task,goes fine
	task = &database.Task{}
	sl4 := newStatusLock(key, task, 0, 1, 2, 3)
	sm := newStatusM()
	sl4.Before = sm.before
	sl4.load = sm.load

	ch := make(chan struct{}, 1)
	sl4.After = func(key string, val int, task *database.Task, t time.Time) error {
		sm.m[key] = val
		sm.task = task
		ch <- struct{}{}

		return nil
	}

	err = sl4.Run(func(val int) bool {
		return val == 0
	}, func() error {
		return nil
	}, true)
	if err != nil {
		t.Error(err)
	}

	<-ch

	if v, err := sl4.load(key); err != nil || v != 2 {
		t.Error(err, v)
	}

	if task.Status != database.TaskDoneStatus {
		t.Error(task.Status)
	} else {
		t.Logf("%+v", task)
	}
}
