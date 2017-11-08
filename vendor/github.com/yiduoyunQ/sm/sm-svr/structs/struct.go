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
	Password        string
	DbPrivilegesMap map[string][]string
	WhiteList       []string
	BlackList       []string
	ReadOnly        bool
	RwSplit         bool
	Shard           bool
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
	ProxyUsers    map[string]*AuthInfo                `json:"proxy_users"`
	ProxyGroups   map[string]*ProxyInfo               `json:"proxy_groups"`
	DatabaseAuth  *DatabaseAuth                       `json:"database_auth,omitempty"`
	DataNodeGroup map[string]map[string]*DatabaseInfo `json:"datanode_group"`
	// DatanodeGroupCnt : status=normal db count
	DataNodeGroupNormalCount map[string]int `json:"datanode_group_normal_count,omitempty"`
}
type ProxyModeInfo struct {
	IsShard   bool   `json:"is_shard,omitempty"`
	IsRwSplit bool   `json:"is_rw_split,omitempty"`
	IsOnly    bool   `json:"is_readonly,omitempty"`
	Datanode  string `json:"datanode,omitempty"`
}
type DatabaseAuth struct {
	DatabaseUsers map[string]*AuthInfo `json:"database_users"`
	//Default              string             `json:"default,omitempty"`
	//Manager              string             `json:"manager,omitempty"`
	ProxyDatabaseUserMap map[string]string `json:"proxy_database_user_map"`
}
type AuthInfo struct {
	Password      string         `json:"password,omitempty"`
	ConnectionMax int            `json:"connection-max,omitempty"`
	ConnectionMin int            `json:"connection-min,omitempty"`
	DbpmName      string         `json:"dbpm_name,omitempty"`
	WhiteList     []string       `json:"white_list,omitempty"`
	BlackList     []string       `json:"black_list,omitempty"`
	ProxyMode     *ProxyModeInfo `json:"proxy_mode,omitempty"`
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
	SlaveIoRunning      string
	SlaveSqlRunning     string
	SecondsBehindMaster int
}
