package store

import (
	"fmt"
	"sort"

	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

const LocalDisk = "local"

type localDisk struct {
	node   *database.Node
	vendor string
	addr   string
	VGName string
}

func NewLocalDisk(vendor, vg, addr string, node *database.Node) Store {
	return &localDisk{
		node:   node,
		addr:   addr,
		vendor: vendor,
		VGName: vg,
	}
}

func (l localDisk) ID() string     { return l.node.ID }
func (l localDisk) Vendor() string { return l.vendor }
func (localDisk) Driver() string   { return LocalDisk }
func (localDisk) Ping() error      { return nil }
func (l localDisk) IdleSize() ([]int, error) {
	// api get local disk size
	list, err := sdk.GetVgList(l.addr)
	if err != nil {
		return nil, err
	}

	total := 0

	for i := range list {
		if l.VGName == list[i].VgName {
			total = list[i].VgSize
			break
		}
	}

	// allocated size
	lvs, err := database.SelectVolumeByVG(l.VGName)
	if err != nil {
		return nil, err
	}

	for i := range lvs {
		total -= lvs[i].Size
	}

	return []int{total}, nil
}

func (localDisk) Insert() error { return nil }

func (localDisk) AddHost(name string, wwwn ...string) error { return nil }
func (localDisk) DelHost(name string, wwwn ...string) error { return nil }

func (l localDisk) Alloc(name string, size int) (string, int, error) {
	idles, err := l.IdleSize()
	if err != nil || len(idles) == 0 {
		return "", 0, err
	}

	sort.Sort(sort.Reverse(sort.IntSlice(idles)))

	if idles[0] < size {
		return "", 0, fmt.Errorf("%s is shortage,%d<%d", l.Vendor(), idles[0], size)
	}

	lv := database.LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       name,
		Size:       size,
		VGName:     l.VGName,
		Driver:     l.Driver(),
		Filesystem: "xfs",
	}

	err = database.InsertLocalVolume(lv)
	if err != nil {
		return "", 0, err
	}

	return lv.ID, size, nil
}

func (localDisk) Recycle(lun int) error {
	return nil
}

func (localDisk) Mapping(host, unit, lun string) error { return nil }
func (localDisk) DelMapping(lun string) error          { return nil }

func (localDisk) AddSpace(id int) (int, error) { return 0, nil }
func (localDisk) EnableSpace(id int) error     { return nil }
func (localDisk) DisableSpace(id int) error    { return nil }
