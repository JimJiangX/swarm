package kvstore

import (
	"net"
	"strings"
	"sync"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

const defaultConsulAddr = "127.0.0.1:8500"

func NewClient(uri string) (Client, error) {
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
		Address: addrs[0],
	}

	c, err := api.NewClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "new consul api Client")
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
	return c.prefix + key
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
				return "", nil, errors.Wrap(errUnavailableKVClient, "get KV client")
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
