package garden

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type eventHander struct {
	ormer database.Ormer
}

func (eh *eventHander) Handle(*cluster.Event) error {
	return nil
}
