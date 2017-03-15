package garden

import (
	"fmt"
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

const (
	_                              = iota           // 0
	statusServcieBuilding          = iota<<4 + _ing // 1
	statusServiceScheduling                         // 2
	statusServiceAllocating                         // 3
	statusServiceContainerCreating                  // 4
	statusInitServiceStarting                       // 5,start contaier and init service
	statusServiceStarting                           // 6,start contaier and start service
	statusServiceStoping                            // 7
	statusServiceBackuping                          // 8
	statusServiceRestoring                          // 9
	statusServiceUsersUpdating                      // 10
	statusServiceScaling                            // 11
	statusServiceConfigUpdating                     // 12
	statusServiceUnitMigrating                      // 13
	statusServiceUnitRebuilding                     // 14
	statusServiceDeleting                           // 15

	_ing    = 0
	_failed = 1
	_done   = 2

	statusServcieBuilt       = statusServcieBuilding + _done
	statusServcieBuildFailed = statusServcieBuilding + _failed

	statusServiceScheduled      = statusServiceScheduling + _done
	statusServiceScheduleFailed = statusServiceScheduling + _failed

	statusServiceAllocated      = statusServiceAllocating + _done
	statusServiceAllocateFailed = statusServiceAllocating + _failed

	statusServiceContainerRunning      = statusServiceContainerCreating + _done
	statusServiceContainerCreateFailed = statusServiceContainerCreating + _failed

	statusInitServiceStarted     = statusInitServiceStarting + _done
	statusInitServiceStartFailed = statusInitServiceStarting + _failed

	statusServiceStarted     = statusServiceStarting + _done
	statusServiceStartFailed = statusServiceStarting + _failed

	statusServiceStoped     = statusServiceStoping + _done
	statusServiceStopFailed = statusServiceStoping + _failed

	statusServiceBackupDone   = statusServiceBackuping + _done
	statusServiceBackupFailed = statusServiceBackuping + _failed

	statusServiceRestored      = statusServiceRestoring + _done
	statusServiceRestoreFailed = statusServiceRestoring + _failed

	statusServiceUsersUpdated      = statusServiceUsersUpdating + _done
	statusServiceUsersUpdateFailed = statusServiceUsersUpdating + _failed

	statusServiceScaled      = statusServiceScaling + _done
	statusServiceScaleFailed = statusServiceScaling + _failed

	statusServiceConfigUpdated      = statusServiceConfigUpdating + _done
	statusServiceConfigUpdateFailed = statusServiceConfigUpdating + _failed

	statusServiceUnitMigrated      = statusServiceUnitMigrating + _done
	statusServiceUnitMigrateFailed = statusServiceUnitMigrating + _failed

	statusServiceUnitRebuilt       = statusServiceUnitRebuilding + _done
	statusServiceUnitRebuildFailed = statusServiceUnitRebuilding + _failed

	statusServiceDeleteFailed = statusServiceDeleting + _failed
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
		set:  ormer.SetServiceStatus,
		cas:  ormer.ServiceStatusCAS,
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
