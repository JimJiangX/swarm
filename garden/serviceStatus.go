package garden

import (
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

const (
	_ing    = 0
	_done   = 1
	_failed = 2

	statusServcieBuilding    = 1<<4 + _ing
	statusServcieBuilt       = statusServcieBuilding + _done
	statusServcieBuildFailed = statusServcieBuilding + _failed

	statusServiceScheduling     = 2<<4 + _ing
	statusServiceScheduled      = statusServiceScheduling + _done
	statusServiceScheduleFailed = statusServiceScheduling + _failed

	statusServiceAllocating     = 3<<4 + _ing
	statusServiceAllocated      = statusServiceAllocating + _done
	statusServiceAllocateFailed = statusServiceAllocating + _failed

	statusServiceContainerCreating     = 4<<4 + _ing
	statusServiceContainerCreated      = statusServiceContainerCreating + _done
	statusServiceContainerCreateFailed = statusServiceContainerCreating + _failed

	statusServiceStarting    = 5<<4 + _ing // start contaier and start service
	statusServiceStarted     = statusServiceStarting + _done
	statusServiceStartFailed = statusServiceStarting + _failed

	statusServiceStoping    = 6<<4 + _ing
	statusServiceStoped     = statusServiceStoping + _done
	statusServiceStopFailed = statusServiceStoping + _failed

	statusServiceBackuping    = 7<<4 + _ing
	statusServiceBackupDone   = statusServiceBackuping + _done
	statusServiceBackupFailed = statusServiceBackuping + _failed

	statusServiceRestoring     = 8<<4 + _ing
	statusServiceRestored      = statusServiceRestoring + _done
	statusServiceRestoreFailed = statusServiceRestoring + _failed

	statusServiceUsersUpdating     = 9<<4 + _ing
	statusServiceUsersUpdated      = statusServiceUsersUpdating + _done
	statusServiceUsersUpdateFailed = statusServiceUsersUpdating + _failed

	statusServiceScaling     = 10<<4 + _ing
	statusServiceScaled      = statusServiceScaling + _done
	statusServiceScaleFailed = statusServiceScaling + _failed

	statusServiceConfigUpdating     = 11<<4 + _ing
	statusServiceConfigUpdated      = statusServiceConfigUpdating + _done
	statusServiceConfigUpdateFailed = statusServiceConfigUpdating + _failed

	statusServiceUnitMigrating     = 12<<4 + _ing
	statusServiceUnitMigrated      = statusServiceUnitMigrating + _done
	statusServiceUnitMigrateFailed = statusServiceUnitMigrating + _failed

	statusServiceUnitRebuilding    = 13<<4 + _ing
	statusServiceUnitRebuilt       = statusServiceUnitRebuilding + _done
	statusServiceUnitRebuildFailed = statusServiceUnitRebuilding + _failed

	statusServiceDeleting     = 14<<4 + _ing
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
		set:  ormer.UpdateServiceStatus,
		cas:  ormer.ServiceStatusCAS,
	}
}
