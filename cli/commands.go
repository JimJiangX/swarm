package cli

import "github.com/urfave/cli"

var (
	commands = []cli.Command{
		{
			Name:      "create",
			ShortName: "c",
			Usage:     "Create a cluster",
			Action:    create,
		},
		{
			Name:      "list",
			ShortName: "l",
			Usage:     "List nodes in a cluster",
			Flags:     []cli.Flag{flTimeout, flDiscoveryOpt},
			Action:    list,
		},
		{
			Name:      "manage",
			ShortName: "m",
			Usage:     "Manage a docker cluster",
			Flags: []cli.Flag{
				flStrategy, flFilter,
				flHosts,
				flLeaderElection, flLeaderTTL, flManageAdvertise,
				flTLS, flTLSCaCert, flTLSCert, flTLSKey, flTLSVerify,
				flRefreshIntervalMin, flRefreshIntervalMax, flFailureRetry, flRefreshRetry,
				flHeartBeat,
				flEnableCors,
				flConfigurePluginAddr,
				flDBDriver, flDBName, flDBAuth, flDBHost, flDBPort, flDBMaxIdle, flDBTablePrefix,
				flCluster, flDiscoveryOpt, flClusterOpt, flRefreshOnNodeFilter, flContainerNameRefreshFilter},
			Action: manage,
		},
		{
			Name:      "join",
			ShortName: "j",
			Usage:     "Join a docker cluster",
			Flags:     []cli.Flag{flJoinAdvertise, flHeartBeat, flTTL, flJoinRandomDelay, flDiscoveryOpt},
			Action:    join,
		},

		{
			Name:      "seedjoin",
			ShortName: "s",
			Usage:     "Join a docker cluster with seed server(version:" + seedversion + ")",
			Flags: []cli.Flag{flJoinAdvertise, flHeartBeat, flTTL, flJoinRandomDelay, flDiscoveryOpt,
				flSeedAddr, flTLS, flTLSCaCert, flTLSCert, flTLSKey, flTLSVerify},
			Action: seedJoin,
		},

		{
			Name:      "configuration",
			ShortName: "cfg",
			Usage:     "Configuration Center Server",
			Flags:     []cli.Flag{flHosts, flTLS, flTLSCaCert, flTLSCert, flTLSKey, flTLSVerify, flScriptDir, flMgmPort, flMgmIP, flDiscoveryOpt},
			Action:    configruation,
		},
	}
)
