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
}

func IsLocalStore(_type string) bool {
	return strings.HasPrefix(_type, LocalStorePrefix)
}

func NewLocalDisk(addr string, node *database.Node) *LocalStore {
	return &LocalStore{
		node: node,
		addr: addr,
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

	fmt.Printf("get vg list: %v\n", list)

	out := make(map[string]int, len(list))

	for i := range list {
		free := list[i].VgSize
		// free := list[i].VgSize * 1000 * 1000

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

	fmt.Printf("get out list: %v\n", out)
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

func (l *LocalStore) Extend(vg, name string, size int) (database.LocalVolume, error) {
	lv, err := database.GetLocalVolume(name)
	if err != nil {
		return lv, err
	}

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
	lv.Size += size

	err = database.UpdateLocalVolume(lv.ID, lv.Size)

	return lv, err
}

func (LocalStore) Recycle(id string) error {

	return database.DeleteLocalVoume(id)
}

func GetVGUsedSize(vg string) (int, error) {
	vgs, err := database.SelectVolumeByVG(vg)
	if err != nil {
		return 0, err
	}

	used := 0

	for i := range vgs {
		used += vgs[i].Size
	}

	return used, nil
}
