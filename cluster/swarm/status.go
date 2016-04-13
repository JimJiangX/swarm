package swarm

const (
	_StatusTaskCreate = iota
	_StatusTaskRunning
	_StatusTaskStop
	_StatusTaskCancel
	_StatusTaskDone
	_StatusTaskTimeout
	_StatusTaskFailed

	_StatusNodeImport = iota
	_StatusNodeInstalling
	_StatusNodeInstalled
	_StatusNodeInstallFailed
	_StatusNodeTesting
	_StatusNodeFailedTest
	_StatusNodeEnable
	_StatusNodeDisable
	_StatusNodeDeregisted

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

	_StatusServiceCreating = iota
	_StatusServiceStarting // start contaier and start service
	_statusServiceStoping
	_StatusServiceDeleting
	_StatusServiceBackuping
	_StatusServiceRestoring
	_StatusServiceNoContent
)
