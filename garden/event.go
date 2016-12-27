package garden

import (
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type eventHander struct {
	ci database.ContainerInterface
}

func (eh eventHander) Handle(event *cluster.Event) error {
	// Something changed - refresh our internal state.
	msg := event.Message

	switch msg.Type {
	case "container":
		action := msg.Action
		// healthcheck events are like 'health_status: unhealthy'
		if strings.HasPrefix(action, "health_status") {
			action = "health_status"
		}

		switch action {
		case "create":
			engine := event.Engine
			c := engine.Containers().Get(msg.ID)
			if c != nil {
				return eh.ci.UnitContainerCreated(c.Info.Name, c.ID, engine.ID, c.HostConfig.NetworkMode, statusContainerCreated)
			}
		default:
			return handleContainerEvent(eh.ci, action, msg.ID)
		}

	case "":
		// docker < 1.10
		switch msg.Status {
		case "create":
			engine := event.Engine

			c := engine.Containers().Get(msg.ID)
			if c != nil {
				return eh.ci.UnitContainerCreated(c.Info.Name, c.ID, engine.ID, c.HostConfig.NetworkMode, statusContainerCreated)
			}

		default:
			return handleContainerEvent(eh.ci, msg.Status, msg.ID)
		}
	}

	return nil
}

func handleContainerEvent(ci database.ContainerInterface, action, ID string) error {
	state := 0

	switch action {
	//case "die", "kill", "oom", "pause", "start", "restart", "stop", "unpause", "rename", "update", "health_status":
	//e.refreshContainer(msg.ID, true)

	case "start", "unpause":
		state = statusContainerRunning

	case "pause":
		state = statusContainerPaused

	case "stop", "kill", "oom":
		state = statusContainerExited

	case "restart":
		state = statusContainerRestarted

	case "die":
		state = statusContainerDead

	default:
		return nil
	}

	return ci.SetUnitByContainer(ID, state)
}
