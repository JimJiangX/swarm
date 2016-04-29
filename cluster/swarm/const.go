package swarm

import "github.com/docker/swarm/cluster/swarm/store"

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

const (
	_MysqlType         = "upsql"          // cluster_type,networking_type
	_ProxyType         = "proxy"          // cluster_type,networking_type
	_SwitchManagerType = "switch_manager" // cluster_type,networking_type

	_SSD          = "SSD_VG"
	_HDD          = "HDD"
	_HDD_VG_Label = "HDD_VG"
	_SSD_VG_Lable = "SSD_VG"

	_SAN_HBA_WWN_Lable = "HBA_WWN"

	_Internal_NIC_Lable = "INT_NIC"
	_External_NIC_Lable = "EXT_NIC"
	_Admin_NIC_Lable    = "ADM_NIC"
)

var (
	supportedServiceTypes = []string{_MysqlType, _ProxyType, _SwitchManagerType}
	supportedStoreTypes   = []string{store.LocalDiskStore, store.SANStore}
)
