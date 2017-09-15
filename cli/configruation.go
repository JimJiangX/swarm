package cli

import (
	"crypto/tls"
	"errors"
	"path/filepath"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/plugin/parser"
	"github.com/urfave/cli"
)

func loadTLSConfigFromContext(c *cli.Context) (*tls.Config, error) {
	// If either --tls or --tlsverify are specified, load the certificates.
	if c.Bool("tls") || c.Bool("tlsverify") {
		if !c.IsSet("tlscert") || !c.IsSet("tlskey") {
			return nil, errors.New("--tlscert and --tlskey must be provided when using --tls")
		}

		if c.Bool("tlsverify") && !c.IsSet("tlscacert") {
			return nil, errors.New("--tlscacert must be provided when using --tlsverify")
		}

		return loadTLSConfig(
			c.String("tlscacert"),
			c.String("tlscert"),
			c.String("tlskey"),
			c.Bool("tlsverify"))
	}

	// Otherwise, if neither --tls nor --tlsverify are specified, abort if
	// the other flags are passed as they will be ignored.
	if c.IsSet("tlscert") || c.IsSet("tlskey") || c.IsSet("tlscacert") {
		return nil, errors.New("--tlscert, --tlskey and --tlscacert require the use of either --tls or --tlsverify")
	}

	return nil, nil
}

func configruation(c *cli.Context) {
	tlsConfig, err := loadTLSConfigFromContext(c)
	if err != nil {
		log.Fatal(err)
	}

	mgmIP := c.String("mgmIP")
	mgmPort := c.Int("mgmPort")

	dir, err := filepath.Abs(c.String("script"))
	if err != nil {
		log.Fatalf("fail to get script dir by '%s'", c.String("script"))
	}

	uri := getDiscovery(c)
	if uri == "" {
		log.Fatalf("discovery required to manage a cluster. See '%s manage --help'.", c.App.Name)
	}

	kvpath := filepath.Join(uri, leaderElectionPath)

	kvClient, err := kvstore.NewClient(uri, getDiscoveryOpt(c))
	if err != nil {
		log.Fatalf("fail to connect to kv store:'%s',%+v", uri, err)
	}

	// see https://github.com/codegangsta/cli/issues/160
	hosts := c.StringSlice("host")
	if c.IsSet("host") || c.IsSet("H") {
		hosts = hosts[1:]
	}

	server := api.NewServer(hosts, tlsConfig)

	server.SetHandler(parser.NewRouter(kvClient, kvpath, dir, mgmIP, mgmPort))

	log.Fatal(server.ListenAndServe())
}
