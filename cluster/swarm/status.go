package swarm

const (
	_StatusTaskCreate = iota
	_StatusTaskRunning
	_StatusTaskStop
	_StatusTaskCancel
	_StatusTaskDone
	_StatusTaskTimeout
	_StatusTaskFailed
)

const (
	_StatusNodeImport = iota
	_StatusNodeInstalling
	_StatusNodeInstalled
	_StatusNodeInstallFailed
	_StatusNodeTesting
	_StatusNodeFailedTest
	_StatusNodeEnable
	_StatusNodeDisable
	_StatusNodeDeregisted
)

const (
	_StatusUnitAllocted = iota
	_StatusUnitCreating
	_StatusUnitStarting // start contaier and start service
	_statusUnitStoping
	_StatusUnitMigrating
	_StatusUnitRebuilding
	_StatusUnitDeleting
	_StatusUnitBackuping
	_StatusUnitRestoring
	_StatusUnitNoContent
)

const (
	_StatusServiceInit = iota
	_StatusServcieBuilding
	_StatusServiceAlloction
	_StatusServiceAlloctionFailed
	_StatusServiceCreating
	_StatusServiceCreateFailed
	_StatusServiceStarting // start contaier and start service
	_StatusServiceStartFailed
	_statusServiceStoping
	_statusServiceStopFaied
	_StatusServiceDeleting
	_StatusServiceDeleteFailed
	_StatusServiceBackuping
	_StatusServiceRestoring
	_StatusServiceRestoreFailed
	_StatusServiceNoContent
)
