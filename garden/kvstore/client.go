package kvstore

import (
	"net"
	"sync"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

const defaultConsulAddr = "127.0.0.1:8500"

func NewClient(config *api.Config) (Client, error) {
	c, err := newConsulClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "new consul api Client")
	}

	_, port, err := net.SplitHostPort(config.Address)
	if err != nil {
		return nil, errors.Wrap(err, "split host port:"+config.Address)
	}

	leader, peers, err := c.getStatus(port)
	if err != nil {
		return nil, err
	}

	kvc := &kvClient{
		lock:   new(sync.RWMutex),
		port:   port,
		leader: leader,
		peers:  peers,
		config: *config,
		agents: make(map[string]kvClientAPI, 10),
	}

	kvc.agents[config.Address] = c
	kvc.config.Address = ""

	return kvc, nil
}

type kvClient struct {
	lock   *sync.RWMutex
	port   string
	leader string
	peers  []string
	agents map[string]kvClientAPI
	config api.Config
}

func (c *kvClient) getLeader() string {
	c.lock.Lock()
	defer c.lock.Unlock()

	for addr, client := range c.agents {
		leader, peers, err := client.getStatus(c.port)
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

			client, err := newConsulClient(&config)
			if err != nil {
				delete(c.agents, addr)
				continue
			}

			leader, peers, err := client.getStatus(c.port)
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

func (c *kvClient) getClient(addr string) (string, kvClientAPI, error) {
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

	client, err := newConsulClient(&config)
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
