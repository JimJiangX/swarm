package store

import (
	"fmt"
	"strings"

	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

const (
	LocalDiskStore = "local"
	SANStore       = "san"
)

type localDisk struct {
	node *database.Node
	addr string
	VGs  []VG
}

type VG struct {
	Vendor string
	Name   string
}

func NewLocalDisk(addr string, node *database.Node, vgs []VG) Store {
	return &localDisk{
		node: node,
		addr: addr,
		VGs:  vgs,
	}
}

func (l localDisk) ID() string { return l.node.ID }
func (l localDisk) Vendor() string {
	vendor := ""
	for i := range l.VGs {
		vendor += l.VGs[i].Vendor + ";"
	}

	return vendor
}

func (localDisk) Driver() string { return LocalDiskStore }
func (localDisk) Ping() error    { return nil }
func (l localDisk) IdleSize() (map[string]int, error) {
	// api get local disk size
	list, err := sdk.GetVgList(l.addr)
	if err != nil {
		return nil, err
	}

	out := make(map[string]int, len(list))

	for i := range list {
		free := list[i].VgSize

		// allocated size
		lvs, err := database.SelectVolumeByVG(list[i].VgName)
		if err != nil {
			return nil, err
		}

		for i := range lvs {
			free -= lvs[i].Size
		}

		out[list[i].VgName] = free
	}

	return out, nil
}

func (localDisk) Insert() error { return nil }

func (localDisk) AddHost(name string, wwwn ...string) error { return nil }
func (localDisk) DelHost(name string, wwwn ...string) error { return nil }

// name with suffix by vgName
func (l localDisk) Alloc(name string, size int) (string, int, error) {
	idles, err := l.IdleSize()
	if err != nil || len(idles) == 0 {
		return "", 0, err
	}

	max := 0
	vgName := ""

	for key, val := range idles {

		if strings.HasSuffix(name, key) {
			vgName = key
			max = val
			break
		}

		if val > max {
			vgName = key
			max = val
		}
	}

	if max < size {
		return "", 0, fmt.Errorf("%s is shortage,%d<%d", l.Vendor(), max, size)
	}

	lv := database.LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       name,
		Size:       size,
		VGName:     vgName,
		Driver:     l.Driver(),
		Filesystem: "xfs",
	}

	err = database.InsertLocalVolume(lv)
	if err != nil {
		return "", 0, err
	}

	return lv.ID, size, nil
}

func (localDisk) Recycle(id string, _ int) error {

	return database.DeleteLocalVoume(id)
}

func (localDisk) Mapping(host, unit, lun string) error { return nil }
func (localDisk) DelMapping(lun string) error          { return nil }

func (localDisk) AddSpace(id int) (int, error) { return 0, nil }
func (localDisk) EnableSpace(id int) error     { return nil }
func (localDisk) DisableSpace(id int) error    { return nil }
