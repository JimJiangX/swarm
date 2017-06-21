package garden

import (
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type eventHander struct {
	once *sync.Once
	ci   database.ContainerIface
}

// NewEventHandler is exported.
//
func NewEventHandler(ormer database.Ormer) *eventHander {
	return &eventHander{
		once: new(sync.Once),
		ci:   ormer,
	}
}

func getContainerNameFromInfo(c types.ContainerJSON) string {
	name := c.Name
	parts := strings.Split(name, "/")
	if v := parts[len(parts)-1]; v != "" {
		name = v
	}

	return name
}

// 当swarm 成为 master时， 执行一次 MarkRunningTasks，把 Task.Status == TaskRunningStatus 标记为 TaskUnknownStatus，
// 当关联的是 Service时，Service.Status ++1。
// 依据 event 类型，更新 Unit 与 Container 的关联，写入数据库。
func (eh eventHander) Handle(event *cluster.Event) (err error) {
	// Something changed - refresh our internal state.
	eh.once.Do(func() {
		logrus.Info("mark running tasks")

		err := eh.ci.MarkRunningTasks()
		if err != nil {
			logrus.Errorf("%+v", err)
		}
	})

	msg := event.Message

	switch msg.Type {
	case "container":
		action := msg.Action
		// healthcheck events are like 'health_status: unhealthy'
		if strings.HasPrefix(action, "health_status") {
			action = "health_status"
		}

		switch action {
		case "create", "rename":
			engine := event.Engine
			c := engine.Containers().Get(msg.ID)
			if c != nil {
				name := getContainerNameFromInfo(c.Info)
				err = eh.ci.UnitContainerCreated(name, c.ID, engine.ID, c.HostConfig.NetworkMode, statusContainerCreated)
			}
		default:
			err = handleContainerEvent(eh.ci, action, msg.ID)
		}

	case "":
		// docker < 1.10
		switch msg.Status {
		case "create", "rename":
			engine := event.Engine

			c := engine.Containers().Get(msg.ID)
			if c != nil {
				name := getContainerNameFromInfo(c.Info)
				err = eh.ci.UnitContainerCreated(name, c.ID, engine.ID, c.HostConfig.NetworkMode, statusContainerCreated)
			}

		default:
			err = handleContainerEvent(eh.ci, msg.Status, msg.ID)
		}
	}

	if err != nil {
		logrus.Errorf("%+v\n%#v", err, msg)
	}

	return err
}

func handleContainerEvent(ci database.ContainerIface, action, ID string) error {
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
