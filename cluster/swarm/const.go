package swarm

import (
	"github.com/docker/swarm/cluster/swarm/storage"
	"github.com/yiduoyunQ/sm/sm-svr/consts"
)

const (
	statusTaskCreate = iota
	statusTaskRunning
	statusTaskStop
	statusTaskCancel
	statusTaskDone
	statusTaskTimeout
	statusTaskFailed
)

const (
	nodeInstallTask   = "node_install"
	imageLoadTask     = "image_load"
	unitMigrateTask   = "unit_migrate"
	unitRebuildTask   = "unit_rebuild"
	unitRestoreTask   = "unit_restore"
	serviceCreateTask = "service_create"
	backupAutoTask    = "backup_auto"
	backupManualTask  = "backup_manual"
)

const (
	statusNodeImport = iota
	statusNodeInstalling
	statusNodeInstalled
	statusNodeInstallFailed
	statusNodeTesting
	statusNodeFailedTest
	statusNodeEnable
	statusNodeDisable

	statusNodeSSHLoginFailed
	statusNodeSCPFailed
	statusNodeSSHExecFailed
	statusNodeRegisterFailed
	statusNodeRegisterTimeout
	statusNodeDeregisted
)

// ParseNodeStatus returns the meaning of the number corresponding
func ParseNodeStatus(status int64) string {
	switch status {
	case statusNodeImport:
		return "importing"
	case statusNodeInstalling:
		return "installing"
	case statusNodeInstalled:
		return "install failed"
	case statusNodeTesting:
		return "testing"
	case statusNodeFailedTest:
		return "test failed"
	case statusNodeEnable:
		return "enable"
	case statusNodeDisable:
		return "disable"
	case statusNodeDeregisted:
		return "deregister"
	default:
	}

	return ""
}

const (
	statusUnitNoContent = iota

	statusUnitAllocting
	statusUnitAllocted
	statusUnitAlloctionFailed

	statusUnitCreating
	statusUnitCreated
	statusUnitCreateFailed

	statusUnitStarting // start contaier and start service
	statusUnitStarted
	statusUnitStartFailed

	statusUnitStoping
	statusUnitStoped
	statusUnitStopFailed

	statusUnitMigrating
	statusUnitMigrated
	statusUnitMigrateFailed

	statusUnitRebuilding
	statusUnitRebuilt
	statusUnitRebuildFailed

	statusUnitDeleting

	statusUnitBackuping
	statusUnitBackuped
	statusUnitBackupFailed

	statusUnitRestoring
	statusUnitRestored
	statusUnitRestoreFailed
)

const (
	statusServiceInit = iota
	statusServcieBuilding
	statusServiceAllocting
	statusServiceAlloctionFailed
	statusServiceCreating
	statusServiceCreateFailed
	statusServiceStarting // start contaier and start service
	statusServiceStartFailed
	statusServiceStoping
	statusServiceStopFailed
	statusServiceDeleting
	statusServiceDeleteFailed
	statusServiceBackuping
	statusServiceRestoring
	statusServiceRestoreFailed
	statusServiceNoContent
)

const (
	_MysqlType         = "upsql" // cluster_type,networking_type
	_UpsqlType         = "upsql"
	_ProxyType         = "proxy"          // cluster_type,networking_type
	_SwitchManagerType = "switch_manager" // cluster_type,networking_type

	_ImageUpsql         = "upsql"
	_ImageProxy         = "upproxy"
	_ImageSwitchManager = "switch_manager"

	_UnitRole_Master        = "master"
	_UnitRole_SwitchManager = "switch_manager"

	_SSD               = "SSD"
	_HDD               = "HDD"
	_HDD_VG_Label      = "HDD_VG"
	_SSD_VG_Label      = "SSD_VG"
	_HDD_VG_Size_Label = "HDD_VG_SIZE"
	_SSD_VG_Size_Label = "SSD_VG_SIZE"

	_SAN_VG = "_SAN_VG"

	_SAN_HBA_WWN_Lable = "HBA_WWN"

	_Internal_NIC_Lable = "INT_NIC"
	_External_NIC_Lable = "EXT_NIC"
	_Admin_NIC_Lable    = "ADM_NIC"

	_NodesNetworking          = "nodes_networking"
	_ContainersNetworking     = "internal_access_networking"
	_ExternalAccessNetworking = "external_access_networking"

	_NetworkingLabelKey      = "upm.ip"
	_ContainerIPLabelKey     = "container_ip"
	_ProxyNetworkingLabelKey = "upm.proxyip"

	_ImageIDInRegistryLabelKey = "registry.imageID"

	_ContainerKVKeyPrefix = "DBAAS/Conatainers/"
)

var (
	supportedServiceTypes = []string{_MysqlType, _UpsqlType, _ProxyType, _SwitchManagerType}
	supportedStoreTypes   = []string{storage.LocalStorePrefix, storage.LocalStorePrefix + ":SSD", storage.LocalStorePrefix + ":HDD", storage.SANStore, storage.HITACHI, storage.HUAWEI, "nfs", "NFS"}
	supportedStoreNames   = []string{"DAT", "LOG", "CNF", "BACKUP"}
)

const (
	_User_DB_Role          = "db"
	_User_DBA_Role         = "cup_dba"
	_User_Application_Role = "ap"
	_User_Monitor_Role     = "mon"
	_User_Replication_Role = "repl"
	_User_Check_Role       = "check"

	User_Type_DB    = consts.Type_Db
	User_Type_Proxy = consts.Type_Proxy

	_DB_Type_M       = consts.Type_M
	_DB_Type_M_SB    = consts.Type_M_SB
	_DB_Type_M_SB_SL = consts.Type_M_SB_SL
)
