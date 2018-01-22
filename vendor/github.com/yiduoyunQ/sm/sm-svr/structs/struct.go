package structs

type MgmPost struct {
	DbaasType           string                             `json:"dbaas-type"`
	DbRootUser          string                             `json:"db-root-user"`
	DbRootPassword      string                             `json:"db-root-password"`
	DbReplicateUser     string                             `json:"db-replicate-user"`
	DbReplicatePassword string                             `json:"db-replicate-password"`
	SwarmApiVersion     string                             `json:"swarm-api-version,omitempty"`
	ProxyGroups         map[string]*ProxyInfo              `json:"proxy_groups"`
	Users               []User                             `json:"users"`
	DataNode            map[string]map[string]DatabaseInfo `json:"data-node"`
}
type User struct {
	Id              string
	UserName        string
	AuthType        string
	Password        string
	DbpmName        string
	DbPrivilegesMap map[string][]string
	WhiteList       []string
	BlackList       []string
	ReadOnly        bool
	RwSplit         bool
	Shard           bool
}

type PutUserRequest struct {
	Id              string
	UserName        string
	AuthType        string
	Password        *string             `json:"Password,omitempty"`
	DbpmName        *string             `json:"DbpmName,omitempty"`
	DbPrivilegesMap map[string][]string `json:"DbPrivilegesMap,omitempty"`
	WhiteList       []string            `json:"WhiteList,omitempty"`
	BlackList       []string            `json:"BlackList,omitempty"`
	ReadOnly        *bool               `json:"ReadOnly,omitempty"`
	RwSplit         *bool               `json:"RwSplit,omitempty"`
	Shard           *bool               `json:"Shard,omitempty"`
}

type ProxyInfo struct {
	Id            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	Ip            string `json:"ip,omitempty"`
	Port          string `json:"port,omitempty"`
	ClientAddress string `json:"cli-address,omitempty"`
	ProxyAddress  string `json:"proxy-address,omitempty"`
	StartupTime   string `json:"startup-time,omitempty"`
	Status        int    `json:"status"`
	ActiveTime    int64  `json:"active-time,omitempty"`
}

type Topology struct {
	Version       string                              `json:"version,omitempty"`
	ProxyMode     *ProxyModeInfo                      `json:"proxy_mode,omitempty"`
	ProxyUsers    map[string]*ProxyAuthInfo           `json:"proxy_users"`
	ProxyGroups   map[string]*ProxyInfo               `json:"proxy_groups"`
	DatabaseAuth  *DatabaseAuth                       `json:"database_auth,omitempty"`
	DataNodeGroup map[string]map[string]*DatabaseInfo `json:"datanode_group"`
	// DatanodeGroupCnt : status=normal db count
	DataNodeGroupNormalCount map[string]int `json:"datanode_group_normal_count,omitempty"`
}

type DatabaseAuth struct {
	DatabaseUsers map[string]*DatabaseAuthInfo `json:"database_users"`
	//Default              string             `json:"default,omitempty"`
	//Manager              string             `json:"manager,omitempty"`
	ProxyDatabaseUserMap map[string]string `json:"proxy_database_user_map"`
}

type ProxyModeInfo struct {
	IsShard   bool   `json:"is_shard,omitempty"`
	IsRwSplit bool   `json:"is_rw_split,omitempty"`
	IsOnly    bool   `json:"is_readonly,omitempty"`
	Datanode  string `json:"datanode,omitempty"`
}

type AuthInfo struct {
	Max int `json:"max,omitempty"`
	Min int `json:"min,omitempty"`

	AuthType   string
	AuthPlugin string `json:"auth_plugin,omitempty"`
	Password   string `json:"password,omitempty"`
	HashPwd    string `json:"hash_pwd,omitempty"`
	DbpmName   string `json:"dbpm_name,omitempty"`

	WhiteList []string `json:"white_list,omitempty"`
	BlackList []string `json:"black_list,omitempty"`

	Mode ProxyModeInfo `json:"proxy_mode,omitempty"`
}

type ProxyAuthInfo struct {
	AuthInfo

	Supervise  int    `json:"supervise,omitempty"`
	SharedFile string `json:"shard_file,omitempty"`
}

type DatabaseAuthInfo struct {
	// TODO:split database user
	IdleFreeTimeout     int `json:"idle_free_timeout,omitempty"`
	IdleExchangeTimeout int `json:"idle_exchange_timeout,omitempty"`
	IdelCheckTimeout    int `json:"idle_check_timeout,omitempty"`
	PreparedSize        int `json:"prepared_size,omitempty"`

	AuthInfo
}

type DatabaseInfo struct {
	Ip           string        `json:"ip"`
	Port         int           `json:"port,omitempty"`
	Status       string        `json:"status,omitempty"`
	Type         string        `json:"type,omitempty"`
	DatabaseAuth *DatabaseAuth `json:"database_auth,omitempty"`
}

type SlaveStatus struct {
	SlaveIoState        string
	MasterIp            string
	MasterPort          int
	MasterLogFile       string
	ReadMasterLogPos    int
	RelayMasterLogFile  string
	SlaveIoRunning      bool
	SlaveSqlRunning     bool
	SecondsBehindMaster int
}
