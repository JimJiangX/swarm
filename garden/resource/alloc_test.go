package resource

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
)

func TestFindIdleCPUs(t *testing.T) {
	want := "2,4,6,7"
	got, err := findIdleCPUs([]string{"0,1", "5,8", "3", "9"}, 10, 4)
	if err != nil {
		t.Error(err)
	}
	if got != want {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	want = "2,4,6,10"
	got, err = findIdleCPUs([]string{"0,1", "5,8", "3", "7,9"}, 11, 4)
	if err != nil {
		t.Error(err)
	}
	if got != want {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	want = "0,10,11,12"
	got, err = findIdleCPUs([]string{"1-9", "13"}, 14, 4)
	if err != nil {
		t.Error(err)
	}
	if got != want {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	got, err = findIdleCPUs([]string{"0,1", "5,8", "3", "7,9"}, 11, 5)
	if err == nil {
		t.Errorf("error expected,but got '%s'", got)
	}
}

func TestAlloctCPUMemory(t *testing.T) {
	config := cluster.ContainerConfig{}
	containers := []*cluster.Container{
		&cluster.Container{
			Config: &cluster.ContainerConfig{
				HostConfig: container.HostConfig{
					Resources: container.Resources{
						CpusetCpus: "1-5,15",
						Memory:     1 << 22,
					}}}},
		&cluster.Container{
			Config: &cluster.ContainerConfig{
				HostConfig: container.HostConfig{
					Resources: container.Resources{
						CpusetCpus: "8-12",
						Memory:     1 << 22,
					}}}},
	}
	nd := node.Node{
		Containers:  cluster.Containers(containers),
		UsedMemory:  8 << 30,
		UsedCpus:    11,
		TotalMemory: 1 << 35,
		TotalCpus:   16,
	}

	actor := NewAllocator(nil, nil)
	sets, err := actor.AlloctCPUMemory(&config, &nd, 5, 1<<34, nil)
	if err != nil {
		t.Error(err, sets)
	}

	if err == nil && (sets != "0,6,7,13,14" ||
		sets != config.HostConfig.CpusetCpus ||
		config.HostConfig.Memory != 1<<34) {
		t.Error(sets, config.HostConfig.CpusetCpus, config.HostConfig.Memory)
	}

	sets, err = actor.AlloctCPUMemory(&config, &nd, 2, 1<<34, []string{"0-13"})
	if err == nil {
		t.Errorf("error expected,but got '%s'", sets)
	} else {
		t.Log(err)
	}

	sets, err = actor.AlloctCPUMemory(&config, &nd, 3, 1<<35-8<<30+1, []string{"0-11"})
	if err == nil {
		t.Errorf("error expected,but got '%s'", sets)
	} else {
		t.Log(err)
	}
}
