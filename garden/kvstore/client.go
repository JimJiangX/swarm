package kvstore

import (
	stderrors "errors"
	"net"
	"sync"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

const defaultConsulAddr = "127.0.0.1:8500"

var errUnavailableKVClient = stderrors.New("non-available consul client")

type Client interface {
	GetHorusAddr() (string, error)
}

func NewClient(config api.Config) (Client, error) {
	_, port, err := net.SplitHostPort(config.Address)
	if err != nil {
		return nil, errors.Wrap(err, "split host port:"+config.Address)
	}

	c, err := api.NewClient(&config)
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
		leader: leader,
		peers:  peers,
		config: config,
		agents: make(map[string]*api.Client, 10),
	}

	kvc.agents[config.Address] = c
	kvc.config.Address = ""

	return kvc, nil
}

func getStatus(client *api.Client, port string) (string, []string, error) {
	leader, err := client.Status().Leader()
	if err != nil {
		return "", nil, errors.Wrap(err, "get consul leader")
	}

	host, _, err := net.SplitHostPort(leader)
	if err != nil {
		return "", nil, errors.Wrap(err, "split host port:"+leader)
	}
	leader = net.JoinHostPort(host, port)

	peers, err := client.Status().Peers()
	if err != nil {
		return "", nil, errors.Wrap(err, "get consul peers")
	}

	addrs := make([]string, 0, len(peers))
	for _, peer := range peers {
		host, _, err := net.SplitHostPort(peer)
		if err != nil {
			continue
		}

		addrs = append(addrs, net.JoinHostPort(host, port))
	}

	return leader, addrs, nil
}

type kvClient struct {
	lock   *sync.RWMutex
	port   string
	leader string
	peers  []string
	agents map[string]*api.Client
	config api.Config
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
