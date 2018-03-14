package garden

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
	statusServiceExecStart                          // 9
	statusServiceRestoring                          // 10
	statusServiceUsersUpdating                      // 11
	statusServiceScaling                            // 12
	statusServiceConfigUpdating                     // 13
	statusServiceUnitMigrating                      // 14
	statusServiceUnitRebuilding                     // 15
	statusServiceImageUpdating                      // 16
	statusServiceResourceUpdating                   // 17
	statusServiceVolumeExpanding                    // 18
	statusServiceNetworkUpdating                    // 19
	statusServiceComposing                          // 20
	statusServiceDeleting                           // 21
	statusServiceDeploying                          // 22

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

	statusServiceExecDone   = statusServiceExecStart + _done
	statusServiceExecFailed = statusServiceExecStart + _failed

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

	statusServiceImageUpdated      = statusServiceImageUpdating + _done
	statusServiceImageUpdateFailed = statusServiceImageUpdating + _failed

	statusServiceResourceUpdated      = statusServiceResourceUpdating + _done
	statusServiceResourceUpdateFailed = statusServiceResourceUpdating + _failed

	statusServiceVolumeExpanded     = statusServiceVolumeExpanding + _done
	statusServiceVolumeExpandFailed = statusServiceVolumeExpanding + _failed

	statusServiceNetworkUpdated      = statusServiceNetworkUpdating + _done
	statusServiceNetworkUpdateFailed = statusServiceNetworkUpdating + _failed

	statusServiceDeleteFailed = statusServiceDeleting + _failed

	statusServiceDeployed     = statusServiceDeploying + _done
	statusServiceDeployFailed = statusServiceDeploying + _failed
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
