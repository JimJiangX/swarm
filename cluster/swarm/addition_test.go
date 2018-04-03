package swarm

import (
	"strconv"
	"testing"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler"
)

func TestListEngines(t *testing.T) {
	c := Cluster{
		scheduler:         new(scheduler.Scheduler),
		engines:           make(map[string]*cluster.Engine, 10),
		pendingEngines:    make(map[string]*cluster.Engine, 10),
		pendingContainers: make(map[string]*pendingContainer, 10),
	}

	for i := 0; i < 10; i++ {
		s := strconv.Itoa(i)
		id, name := "engineID"+s, "host"+s

		c.engines[id] = &cluster.Engine{
			ID:   id,
			Name: name,
		}
	}

	for i := 10; i < 20; i++ {
		s := strconv.Itoa(i)
		id, name := "engineID"+s, "host"+s

		c.pendingEngines[s] = &cluster.Engine{
			ID:   id,
			Name: name,
		}
	}

	engines := c.ListEngines()
	if len(engines) != 10 {
		t.Errorf("expect %d but got %d", 10, len(engines))
	}

	engines = c.ListEngines("host3")
	if len(engines) != 1 {
		t.Errorf("expect %d but got %d", 1, len(engines))
	}
	if engines[0].ID != "engineID3" || engines[0].Name != "host3" {
		t.Errorf("got unexpected engine,%v", engines[0])
	}

	engines = c.ListEngines("host13")
	if len(engines) != 0 {
		t.Errorf("expect %d but got %d", 0, len(engines))
	}

	list := []string{"host0", "host5", "host10", "host20", "engineID1", "engineID10", "engineID11", "engineID19"}
	engines = c.ListEngines(list...)
	if len(engines) != 3 {
		t.Errorf("expect %d but got %d", 3, len(engines))
	}
}
