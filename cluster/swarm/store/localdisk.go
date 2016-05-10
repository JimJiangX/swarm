package store

import (
	"fmt"

	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
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

func (localDisk) Driver() string { return LocalStoreDriver }
func (localDisk) Ping() error    { return nil }
func (l localDisk) IdleSize() (map[string]int, error) {
	// api get local disk size
	list, err := sdk.GetVgList(l.addr)
	if err != nil {
		return nil, err
	}

	fmt.Println("get vg list: %v", list)

	out := make(map[string]int, len(list))

	for i := range list {
		// free := list[i].VgSize * 1024 * 1024
		free := list[i].VgSize * 1000 * 1000

		// allocated size
		lvs, err := database.SelectVolumeByVG(list[i].VgName)
		if err != nil {
			return nil, err
		}

		fmt.Println("get lvs list: %v", lvs)

		for i := range lvs {
			free -= lvs[i].Size
		}

		out[list[i].VgName] = free
	}
	fmt.Println("get out list: %v", out)
	return out, nil
}

func (localDisk) Insert() error { return nil }

func (localDisk) AddHost(name string, wwwn ...string) error { return nil }
func (localDisk) DelHost(name string, wwwn ...string) error { return nil }

func (l localDisk) Alloc(name, vg string, size int) (string, int, error) {
	idles, err := l.IdleSize()
	if err != nil || len(idles) == 0 {
		return "", 0, err
	}

	vgsizes, ok := idles[vg]

	if !ok {
		return "", 0, fmt.Errorf("%s:don't get vg size", vg)
	}

	if vgsizes < size {
		return "", 0, fmt.Errorf("%s is shortage,%d<%d", vg, vgsizes, size)
	}

	lv := database.LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       name,
		Size:       size,
		VGName:     vg,
		Driver:     l.Driver(),
		Filesystem: DefaultFilesystemType,
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
