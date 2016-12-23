package garden

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type eventHander struct {
	ci      database.ContainerInterface
	cluster cluster.Cluster
}

func (eh eventHander) Handle(event *cluster.Event) error {
	// Something changed - refresh our internal state.
	msg := event.Message

	switch msg.Type {
	case "container":
		return eh.handleContainer(msg.Action, msg.ID)

	case "":
		// docker < 1.10
		return eh.handleContainer(msg.Status, msg.ID)
	}

	return nil
}

func (eh eventHander) handleContainer(action, ID string) error {
	switch action {
	//case "die", "kill", "oom", "pause", "start", "restart", "stop", "unpause", "rename", "update", "health_status":
	//e.refreshContainer(msg.ID, true)

	case "create":
		c := eh.cluster.Container(ID)
		if c != nil {
			return eh.ci.UnitContainerCreated(c.Info.Name, c.ID, c.Engine.ID, c.HostConfig.NetworkMode, statusContainerCreated)
		}

	case "start", "unpause":
		return eh.ci.UpdateUnitByContainer(ID, statusContainerRunning)

	case "pause":
		return eh.ci.UpdateUnitByContainer(ID, statusContainerPaused)

	case "stop", "kill", "oom":
		return eh.ci.UpdateUnitByContainer(ID, statusContainerExited)

	case "restart":
		return eh.ci.UpdateUnitByContainer(ID, statusContainerRestarted)

	case "die":
		return eh.ci.UpdateUnitByContainer(ID, statusContainerDead)

	default:
		//e.refreshContainer(msg.ID, false)
	}

	return nil
}
