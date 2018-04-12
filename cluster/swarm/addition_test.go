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
			IP:   "192.168.3." + s,
		}
	}

	for i := 10; i < 20; i++ {
		s := strconv.Itoa(i)
		id, name := "engineID"+s, "host"+s

		c.pendingEngines[s] = &cluster.Engine{
			ID:   id,
			Name: name,
			IP:   "192.168.3." + s,
		}
	}

	engines := c.ListEngines()
	if len(engines) != 10 {
		t.Errorf("expect %d but got %d", 10, len(engines))
	}

	engines = c.ListEngines("engineID3")
	if len(engines) != 1 {
		t.Errorf("expect %d but got %d", 1, len(engines))
	}

	var e *cluster.Engine
	for _, n := range engines {
		e = n
	}

	if e.ID != "engineID3" || e.Name != "host3" {
		t.Errorf("got unexpected engine,%v", e)
	}

	engines = c.ListEngines("engineID13")
	if len(engines) != 0 {
		t.Errorf("expect %d but got %d", 0, len(engines))
	}

	list := []string{"engineID0", "engineID5", "engineID10", "engineID20", "engineID1", "engineID10", "engineID11", "engineID19"}
	engines = c.ListEngines(list...)
	if len(engines) != 3 {
		t.Errorf("expect %d but got %d", 3, len(engines))
	}
}
