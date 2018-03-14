package kvstore

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/go-connections/tlsconfig"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

const (
	defaultConsulAddr = "127.0.0.1:8500"
	defaultTimeout    = 2 * time.Minute
)

// NewClient returns a consul Client
func NewClient(uri string, options map[string]string) (Client, error) {
	if uri == "" {
		uri = defaultConsulAddr
	}

	var (
		prefix  = ""
		_, uris = parse(uri)
		parts   = strings.SplitN(uris, "/", 2)
		addrs   = strings.Split(parts[0], ",")
	)

	// A custom prefix to the path can be optionally used.
	if len(parts) == 2 {
		prefix = parts[1]
	}

	_, port, err := net.SplitHostPort(addrs[0])
	if err != nil {
		return nil, errors.Wrap(err, "split host port:"+addrs[0])
	}

	config := &api.Config{
		Address:  addrs[0],
		WaitTime: defaultTimeout,
	}

	var tlsConfig *tls.Config
	if options["kv.cacertfile"] != "" && options["kv.certfile"] != "" && options["kv.keyfile"] != "" {
		tlsConfig, err = tlsconfig.Client(tlsconfig.Options{
			CAFile:   options["kv.cacertfile"],
			CertFile: options["kv.certfile"],
			KeyFile:  options["kv.keyfile"],
		})
		if err != nil {
			return nil, err
		}
	}

	return MakeClient(config, prefix, port, tlsConfig)
}

// MakeClient returns a consul kv client
func MakeClient(config *api.Config, prefix, port string, tlsConfig *tls.Config) (*kvClient, error) {
	if tlsConfig != nil {
		config.HttpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		config.Scheme = "https"
	}

	c, err := api.NewClient(config)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	leader, peers, err := getStatus(c, port)
	if err != nil {
		return nil, err
	}

	kvc := &kvClient{
		lock:   new(sync.RWMutex),
		port:   port,
		prefix: prefix,
		leader: leader,
		peers:  peers,
		config: *config,
		agents: make(map[string]*api.Client, 10),
	}

	kvc.agents[config.Address] = c
	kvc.config.Address = ""

	return kvc, nil
}

func parse(rawurl string) (string, string) {
	parts := strings.SplitN(rawurl, "://", 2)

	// nodes:port,node2:port => nodes://node1:port,node2:port
	if len(parts) == 1 {
		return "nodes", parts[0]
	}
	return parts[0], parts[1]
}

type kvClient struct {
	lock   *sync.RWMutex
	prefix string
	port   string
	leader string
	peers  []string
	agents map[string]*api.Client
	config api.Config
}

func (c kvClient) key(key string) string {
	if len(key) > 0 && key[0] == '/' {
		return c.prefix + key
	}

	return c.prefix + "/" + key
}

func (c *kvClient) getLeader() string {
	c.lock.Lock()
	defer c.lock.Unlock()

	for addr, client := range c.agents {
		leader, peers, err := getStatus(client, c.port)
		if err != nil {
			delete(c.agents, addr)
			continue
		}

		c.leader = leader
		c.peers = peers

		return c.leader
	}

	if c.leader == "" {
		addrs := append(c.peers, defaultConsulAddr)

		for _, addr := range addrs {

			config := c.config
			config.Address = addr

			client, err := api.NewClient(&config)
			if err != nil {
				delete(c.agents, addr)
				continue
			}

			leader, peers, err := getStatus(client, c.port)
			if err != nil {
				delete(c.agents, addr)
				continue
			}

			c.leader = leader
			c.peers = peers
			c.agents[addr] = client

			return leader
		}
	}

	return c.leader
}

func (c *kvClient) getClient(addr string) (string, *api.Client, error) {
	if addr == "" {
		// get kv leader client
		c.lock.RLock()

		addr = c.leader
		client := c.agents[addr]

		c.lock.RUnlock()

		if client != nil {
			return addr, client, nil
		}

		if addr == "" {

			addr = c.getLeader()
			if addr == "" {
				return "", nil, errors.WithStack(errUnavailableKVClient)
			}
		}
	}

	c.lock.RLock()
	client := c.agents[addr]

	if client == nil {
		_, _, err := net.SplitHostPort(addr)
		if err != nil {
			addr = net.JoinHostPort(addr, c.port)
			client = c.agents[addr]
		}
	}

	c.lock.RUnlock()

	if client != nil {
		return addr, client, nil
	}

	config := c.config
	config.Address = addr

	client, err := api.NewClient(&config)
	if err != nil {
		return "", nil, errors.Wrap(err, "new consul api Client")
	}

	c.lock.Lock()
	c.agents[addr] = client
	c.lock.Unlock()

	return addr, client, nil
}

func (c *kvClient) checkConnectError(addr string, err error) {
	if err == nil {
		return
	}

	c.lock.Lock()

	if addr == c.leader {
		c.leader = ""
	}
	delete(c.agents, addr)

	c.lock.Unlock()
}
