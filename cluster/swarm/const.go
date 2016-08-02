package swarm

import (
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/yiduoyunQ/sm/sm-svr/consts"
)

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
	_Node_Install_Task   = "node_install"
	_Image_Load_Task     = "image_load"
	_Unit_Migrate_Task   = "unit_migrate"
	_Unit_Rebuild_Task   = "unit_rebuild"
	_Unit_Restore_Task   = "unit_restore"
	_Service_Create_Task = "service_create"
	_Backup_Auto_Task    = "backup_auto"
	_Backup_Manual_Task  = "backup_manual"
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

func ParseNodeStatus(status int64) string {
	switch status {
	case _StatusNodeImport:
		return "importing"
	case _StatusNodeInstalling:
		return "installing"
	case _StatusNodeInstalled:
		return "install failed"
	case _StatusNodeTesting:
		return "testing"
	case _StatusNodeFailedTest:
		return "test failed"
	case _StatusNodeEnable:
		return "enable"
	case _StatusNodeDisable:
		return "disable"
	case _StatusNodeDeregisted:
		return "deregister"
	default:
	}

	return ""
}

const (
	_StatusUnitNoContent = iota
	_StatusUnitAllocted
	_StatusUnitCreating
	_StatusUnitStarting // start contaier and start service
	_statusUnitStoping
	_StatusUnitMigrating
	_StatusUnitRebuilding
	_StatusUnitDeleting
	_StatusUnitBackuping
	_StatusUnitRestoring
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
	_statusServiceStopFailed
	_StatusServiceDeleting
	_StatusServiceDeleteFailed
	_StatusServiceBackuping
	_StatusServiceRestoring
	_StatusServiceRestoreFailed
	_StatusServiceNoContent
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

	_SSD          = "SSD"
	_HDD          = "HDD"
	_HDD_VG_Label = "HDD_VG"
	_SSD_VG_Label = "SSD_VG"

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
)

var (
	supportedServiceTypes = []string{_MysqlType, _UpsqlType, _ProxyType, _SwitchManagerType}
	supportedStoreTypes   = []string{store.LocalStorePrefix, store.LocalStorePrefix + ":SSD", store.LocalStorePrefix + ":HDD", store.SANStore, store.HITACHI, store.HUAWEI, "nfs", "NFS"}
	supportedStoreNames   = []string{"DAT", "LOG", "CNF", "BACKUP"}
)

const (
	_User_DB          = "db"
	_User_DBA         = "cup_dba"
	_User_Application = "ap"
	_User_Monitor     = "mon"
	_User_Replication = "repl"
	_User_Check       = "check"

	_User_Type_DB    = consts.Type_Db
	_User_Type_Proxy = consts.Type_Proxy

	_DB_Type_M       = consts.Type_M
	_DB_Type_M_SB    = consts.Type_M_SB
	_DB_Type_M_SB_SL = consts.Type_M_SB_SL
)
