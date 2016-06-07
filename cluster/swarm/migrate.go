package swarm

import (
	"fmt"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/utils"
)

func (gd *Gardener) UnitRebuild(name string, candidates []string) error {
	table, err := database.GetUnit(name)
	if err != nil {
		return fmt.Errorf("Not Found Unit %s,error:%s", name, err)
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return err
	}

	svc.RLock()
	index, module := 0, structs.Module{}
	filters := make([]string, len(svc.units))
	for i, u := range svc.units {
		filters[i] = u.EngineID
		if u.Name == name {
			index = i
		}
	}
	u := svc.units[index]

	for i := range svc.base.Modules {
		if u.Type == svc.base.Modules[i].Type {
			module = svc.base.Modules[i]
			break
		}
	}
	svc.RUnlock()
	if err != nil {
		return err
	}

	err = u.stopContainer(0)
	if err != nil {
		return err
	}

	config, err := resetContainerConfig(u.container.Config)
	if err != nil {
		return err
	}

	node, err := selectNode(gd, config, module, candidates, filters)
	if err != nil || node == nil {
		return err
	}

	return nil
}

func selectNode(gd *Gardener, config *cluster.ContainerConfig, module structs.Module, list, exclude []string) (*node.Node, error) {
	entry := logrus.WithFields(logrus.Fields{"Module": module.Type})

	num, _type := 1, module.Type
	// TODO:maybe remove tag
	if module.Type == _SwitchManagerType {
		_type = _ProxyType
	}
	filters := gd.listShortIdleStore(module.Stores, _type, num)
	filters = append(filters, exclude...)
	entry.Debugf("[MG] %s,%s,%s:first filters of storage:%s", module.Stores, module.Type, num, filters)

	candidates, err := gd.Scheduler(config, _type, num, list, filters, false, false)
	if err != nil {
		return nil, err
	}

	return candidates[0], nil
}

func resetContainerConfig(config *cluster.ContainerConfig) (*cluster.ContainerConfig, error) {
	//
	ncpu, err := utils.GetCPUNum(config.HostConfig.CpusetCpus)
	if err != nil {
		return nil, err
	}
	clone := cloneContainerConfig(config)
	// reset CpusetCpus
	clone.HostConfig.CpusetCpus = strconv.FormatInt(ncpu, 10)

	return clone, nil
}
