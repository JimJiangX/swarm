package consts

import (
	"time"
)

const (
	// proxy
	ProxyOK = iota
	ProxyTobeClose
	ProxyClose
)

const (
	// status
	StatusOK      = "OK"
	StatusWarning = "Warning"
	StatusError   = "Error"
)

const (
	// global
	ConfigFile   = "/tmp/sm.conf"
	ProxyFile    = "/tmp/proxy.json"
	TopologyFile = "/tmp/topology.json"
	SwarmFile    = "/tmp/swarm.json"
)

const (
	// consul k/v key
	InitKey               = "Init" // "1":initalized
	ProxyKey              = "Proxy"
	TopologyKey           = "Topology"
	SwarmKey              = "Swarm"
	SwarmHostKey          = "SwarmHost"
	ActionKey             = "Action"
	ActionIsolateDbVal    = "isolateDb"
	ActionIsolateProxyVal = "isolatePxy"
	ActionRecoverDbVal    = "recoverDb"
)

const (
	// sm time out
	HealthCheckLoop     = 7
	HealthCheckCheck    = 4
	HealthCheckInterval = 10 * time.Second

	// lock time out
	LockRetryTimes    = 10
	LockRetryInterval = 1 * time.Second

	// dbaas type
	Type_M       = "M"
	Type_M_SB    = "M_SB"
	Type_M_SB_SL = "M_SB_SL"

	// mgm user type
	Type_Proxy = "Proxy"
	Type_Db    = "DB"
)

const (
	// swarm
	FillLine        = "Warning: Using a password on the command line interface can be insecure."
	SwarmUserAgent  = "engine-api-cli-1.0"
	SwarmSocketPath = "/DBAASDAT/upsql.sock"

	// swarm
	DelayRetryTimes      = 3
	DelayRetryInterval   = 3 * time.Second
	DelayRetryTimeout    = 10 * time.Second
	DelayRetryTimeoutAll = 30 * time.Second

	// swarm timeout
	SwarmRetryTimes      = 3
	SwarmRetryInterval   = 3 * time.Second
	SwarmRetryTimeout    = 10 * time.Second
	SwarmRetryTimeoutAll = 30 * time.Second

	SwarmSlaveCheckTimes    = 3
	SwarmSlaveCheckInterval = 3 * time.Second

	SwarmHealthCheckApp         = "/root/check_db"
	SwarmHealthCheckUser        = "check"
	SwarmHealthCheckPassword    = "123.com"
	SwarmHealthCheckConfigFile  = "/DBAASDAT/my.cnf"
	SwarmHealthCheckTimeout     = "5s"
	SwarmHealthCheckReadTimeout = "5s"
)

const (
	// consul host eth
	ConsulBindNetworkName = "bond0"
	ConsulPort            = ":8500"

	// consul time out
	ConsulRetryTimes      = 3
	ConsulRetryInterval   = 3 * time.Second
	ConsulRetryTimeout    = 10 * time.Second
	ConsulRetryTimeoutAll = 30 * time.Second
)

const (
	// ha
	Version_1 = "1"
	Master    = "master"
	StandBy   = "standby"
	Slave     = "slave"
	Normal    = "normal"
	Abnormal  = "abnormal"
)
