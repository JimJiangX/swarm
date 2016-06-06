package store

import (
	"fmt"
	"strings"

	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

type LocalStore struct {
	node *database.Node
	addr string
	VGs  []VG
}

type VG struct {
	Vendor string
	Name   string
}

func IsStoreLocal(_type string) bool {
	return strings.Contains(_type, LocalStorePrefix)
}

func NewLocalDisk(addr string, node *database.Node, vgs []VG) *LocalStore {
	return &LocalStore{
		node: node,
		addr: addr,
		VGs:  vgs,
	}
}

func (l LocalStore) ID() string   { return l.node.ID }
func (LocalStore) Driver() string { return LocalStoreDriver }
func (l LocalStore) IdleSize() (map[string]int, error) {
	// api get local disk size
	list, err := sdk.GetVgList(l.addr)
	if err != nil {
		return nil, err
	}

	fmt.Println("get vg list: %v", list)

	out := make(map[string]int, len(list))

	for i := range list {
		free := list[i].VgSize
		// free := list[i].VgSize * 1000 * 1000

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

func (l LocalStore) Alloc(name, unit, vg string, size int) (database.LocalVolume, error) {
	lv := database.LocalVolume{}
	idles, err := l.IdleSize()
	if err != nil || len(idles) == 0 {
		return lv, err
	}

	vgsize, ok := idles[vg]
	if !ok {
		return lv, fmt.Errorf("doesn't get VG %s", vg)
	}

	if vgsize < size {
		return lv, fmt.Errorf("VG %s is shortage,%d<%d", vg, vgsize, size)
	}

	lv = database.LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       name,
		Size:       size,
		UnitID:     unit,
		VGName:     vg,
		Driver:     l.Driver(),
		Filesystem: DefaultFilesystemType,
	}

	err = database.InsertLocalVolume(lv)
	if err != nil {
		return lv, err
	}

	return lv, nil
}

func (LocalStore) Recycle(id string) error {

	return database.DeleteLocalVoume(id)
}
