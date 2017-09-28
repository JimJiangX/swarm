package garden

import (
	"testing"
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
)

func TestReduceCPUset(t *testing.T) {
	want := "0,2,3,4,5"
	got, err := reduceCPUset("11,2,6,9,3,4,5,0", 5)
	if err != nil {
		t.Error(err)
	}
	if want != got {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	want = "0,1,2,3,4,5,6,9"
	got, err = reduceCPUset("1,1,2,6,9,3,4,5,0,9,3,4,5,0", 8)
	if err != nil {
		t.Error(err)
	}
	if want != got {
		t.Errorf("Unexpected,want '%s' but got '%s'", want, got)
	}

	got, err = reduceCPUset("1,1,2,6,9,3,4,5,0,9,3,4,5,0", 9)
	if err == nil {
		t.Error("error expected")
	}

	t.Log(got, err)
}

var es = database.Service{
	Desc: &database.ServiceDesc{
		ID:              "8238b3ffafd039847856c9561287b6d9",
		ServiceID:       "4a5e1efb84baa7cb595fb18525546664",
		Architecture:    `{"unit_num":3,"mode":"sharding_replication","code":"M:1#S:0"}`,
		ScheduleOptions: `{"UnitRequire":{"require":{"ncpu":1,"memory":2147483648},"volumes":[{"name":"DAT","type":"local:HDD","size":5368709120,"options":null},{"name":"LOG","type":"local:HDD","size":5368709120,"options":null},{"name":"","type":"NFS","size":0,"options":null}],"networks":[{"bandwidth":50}]},"Nodes":{"Networkings":{"de5f219ffc2919caae859f2ed26d8e4d":["91c23b15f4c84857a5fa36a1d9076d53"]},"Clusters":["de5f219ffc2919caae859f2ed26d8e4d"]},"Scheduler":{}}`,
		Replicas:        3,
		NCPU:            1,
		Memory:          2147483648,
		ImageID:         "9ece2d31ffcbbad41ea7a1ce78567754",
		Image:           "redis:3.2.8",
		Volumes:         `[{"name":"DAT","type":"local:HDD","size":5368709120,"options":null},{"name":"LOG","type":"local:HDD","size":5368709120,"options":null},{"name":"","type":"NFS","size":0,"options":null}]`,
		Networks:        `{"NetworkingIDs":{"de5f219ffc2919caae859f2ed26d8e4d":["91c23b15f4c84857a5fa36a1d9076d53"]},"Require":[{"bandwidth":50}]}`,
		Clusters:        "de5f219ffc2919caae859f2ed26d8e4d",
		Options:         `{"port":10000}`,
		Previous:        "",
	},
	ID:            "4a5e1efb84baa7cb595fb18525546664",
	Name:          "c66b3d0961084b2a8138e2fb5e91a7b7",
	DescID:        "8238b3ffafd039847856c9561287b6d9",
	Tag:           "xxxx",
	AutoHealing:   false,
	AutoScaling:   false,
	HighAvailable: false,
	Status:        82,
	CreatedAt:     time.Now(),
	FinishedAt:    time.Now().AddDate(0, 0, 1),
}

func TestUpdateDescByImage(t *testing.T) {
	table := updateDescByImage(es, database.Image{
		ID:      "IMAGEafjoaealjfoajafjaofa",
		Name:    "mysql",
		ImageID: "IMAGEIDafjofalfjappfak",
		Major:   5,
		Minor:   7,
		Patch:   16,
		Build:   0,
	})

	if table.Desc.ID == es.ID ||
		table.DescID == es.DescID ||
		table.Desc.Previous != es.Desc.ID ||
		table.Desc.Image == es.Desc.Image ||
		table.Desc.ImageID == es.Desc.ImageID {
		t.Error(table, es)
	}
}

func TestUpdateDescByResource(t *testing.T) {
	var ncpu, memory int64 = 5, 179418

	table, err := updateDescByResource(es, ncpu, memory)
	if err != nil {
		t.Error(err)
	}

	if table.Desc.ID == es.ID ||
		table.DescID == es.DescID ||
		table.Desc.Previous != es.Desc.ID ||
		table.Desc.NCPU != int(ncpu) ||
		table.Desc.Memory != memory ||
		table.Desc.ScheduleOptions == es.Desc.ScheduleOptions {

		t.Error(table, es)
	}

	//	t.Log(table.Desc.ScheduleOptions)
}

func TestUpdateDescByVolumeReuires(t *testing.T) {
	req := []structs.VolumeRequire{
		{
			Type: "DAT",
			Name: "SAN",
			Size: 7492740840,
		},
		{
			Type: "LOG",
			Name: "local:HDD",
			Size: 74927410840,
		},
	}

	table, err := updateDescByVolumeReuires(es, nil, req)
	if err != nil {
		t.Error(err)
	}

	if table.Desc.ID == es.ID ||
		table.DescID == es.DescID ||
		table.Desc.Previous != es.Desc.ID ||
		table.Desc.Volumes == es.Desc.Volumes ||
		table.Desc.ScheduleOptions == es.Desc.ScheduleOptions {

		t.Error(table, es)
	}

	//	t.Log(table.Desc.Volumes)
	//	t.Log(table.Desc.ScheduleOptions)
}

func TestMergeVolumeRequire(t *testing.T) {
	old := []structs.VolumeRequire{
		{
			Type: "local:HDD",
			Name: "DAT",
			Size: 7492740840,
		},
		{
			Name: "LOG",
			Type: "local:SSD",
			Size: 74927410840,
		},
	}
	req := []structs.VolumeRequire{
		{
			Name: "DAT",
			Size: 7494084079295,
		},
		{
			Name: "LOG",
			Size: 742131392740,
		},

		{
			Name: "XXX",
			Size: 742131392740,
		},
	}

	out, err := mergeVolumeRequire(old, req)
	if err == nil {
		t.Error("error expected")
	}

	t.Log(err)

	out, err = mergeVolumeRequire(old, req[0:2])
	if err != nil {
		t.Error(err)
	}

	t.Log(out)
}

func TestUpdateDescByArch(t *testing.T) {
	table := updateDescByArch(es, structs.Arch{
		Replicas: 5,
		Mode:     "mode",
		Code:     "modexxx990",
	})

	if table.Desc.ID == es.ID ||
		table.DescID == es.DescID ||
		table.Desc.Previous != es.Desc.ID ||
		table.Desc.Replicas != 5 ||
		table.Desc.Architecture == es.Desc.Architecture {
		t.Error(table, es)
	}
}
