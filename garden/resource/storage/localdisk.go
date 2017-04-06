package storage

//import (
//	"strings"
//	"time"

//	"github.com/docker/swarm/garden/database"
//	"github.com/docker/swarm/garden/utils"
//	"github.com/docker/swarm/seed/sdk"
//	"github.com/pkg/errors"
//)

//var defaultTTL = time.Minute * 10

//// LocalStore host local storage,HDD or SSD
//type LocalStore struct {
//	addr string
//	node *database.Node

//	ttl     time.Duration
//	expired time.Time
//	list    []sdk.VgInfo
//}

//// IsLocalStore returns true is _type is local
//func IsLocalStore(_type string) bool {
//	return strings.HasPrefix(_type, LocalStorePrefix)
//}

//// NewLocalDisk returns the node local storage
//func NewLocalDisk(addr string, node *database.Node, ttl time.Duration) (*LocalStore, error) {
//	if ttl == 0 {
//		ttl = defaultTTL
//	}
//	l := &LocalStore{
//		node: node,
//		addr: addr,
//		ttl:  ttl,
//	}

//	list, err := sdk.GetVgList(l.addr)
//	if err == nil {
//		l.list = list
//		l.expired = time.Now().Add(l.ttl)
//	} else {

//		return l, errors.Wrap(err, "new localdisk store")
//	}

//	return l, nil
//}

//// ID returns node ID
//func (l LocalStore) ID() string { return l.node.ID }

//// Driver returns local store driver
//func (LocalStore) Driver() string { return LocalStoreDriver }

//// IdleSize returns local store idle size,include HDD,and SSD maybe
//func (l LocalStore) IdleSize() (map[string]int, error) {
//	if time.Now().After(l.expired) {
//		// api get local disk size
//		list, err := sdk.GetVgList(l.addr)
//		if err != nil {
//			return nil, errors.Wrap(err, l.node.Name+" store idle size")
//		}

//		l.list = list
//		l.expired = time.Now().Add(l.ttl)
//	}

//	out := make(map[string]int, len(l.list))

//	for i := range l.list {
//		free := l.list[i].VgSize
//		// free := list[i].VgSize * 1000 * 1000

//		// allocated size
//		lvs, err := database.ListVolumeByVG(l.list[i].VgName)
//		if err != nil {
//			return nil, errors.Wrap(err, l.node.Name+" store idle size")
//		}

//		for i := range lvs {
//			free -= lvs[i].Size
//		}

//		if free > l.list[i].VgFree {
//			free = l.list[i].VgFree
//		}

//		out[l.list[i].VgName] = free
//	}

//	return out, nil
//}

//// Alloc returns allocated local volume on the host
//func (l LocalStore) Alloc(name, unit, vg string, size int) (database.Volume, error) {
//	lv := database.Volume{}
//	idles, err := l.IdleSize()
//	if err != nil || len(idles) == 0 {
//		return lv, errors.Wrap(err, l.node.Name+" store alloc")
//	}

//	vgsize, ok := idles[vg]
//	if !ok {
//		return lv, errors.Errorf("%s:doesn't get VG %s", l.node.Name, vg)
//	}

//	if vgsize < size {
//		return lv, errors.Errorf("%s:VG %s is shortage,%d<%d", l.node.Name, vg, vgsize, size)
//	}

//	lv = database.LocalVolume{
//		ID:         utils.Generate64UUID(),
//		Name:       name,
//		Size:       size,
//		UnitID:     unit,
//		VGName:     vg,
//		Driver:     l.Driver(),
//		Filesystem: DefaultFilesystemType,
//	}

//	err = database.InsertLocalVolume(lv)
//	if err != nil {
//		return lv, errors.Wrap(err, l.node.Name+" store alloc")
//	}

//	return lv, nil
//}

//// Extend extend a exist VG size
//func (l *LocalStore) Extend(vg, name string, size int) (database.Volume, error) {
//	lv, err := database.GetVolume(name)
//	if err != nil {
//		return lv, errors.Wrap(err, l.node.Name+" VG extend")
//	}

//	idles, err := l.IdleSize()
//	if err != nil || len(idles) == 0 {
//		return lv, errors.Wrap(err, l.node.Name+" VG extend")
//	}

//	vgsize, ok := idles[vg]
//	if !ok {
//		return lv, errors.Errorf("%s doesn't get VG %s", l.node.Name, vg)
//	}

//	if vgsize < size {
//		return lv, errors.Errorf("%s:VG %s is shortage,%d<%d", l.node.Name, vg, vgsize, size)
//	}
//	lv.Size += size

//	err = database.UpdateLocalVolume(lv.ID, lv.Size)
//	if err != nil {
//		return lv, errors.Wrap(err, l.node.Name+" VG extend")
//	}

//	return lv, nil
//}

//// Recycle recycle a volume
//func (l LocalStore) Recycle(id string) error {
//	err := database.DeleteLocalVoume(id)
//	if err != nil {
//		return errors.Wrap(err, l.node.Name+" recycle volume")
//	}

//	return nil
//}

//// GetVGUsedSize returns VG used size
//func GetVGUsedSize(vg string) (int, error) {
//	vgs, err := database.ListVolumeByVG(vg)
//	if err != nil {
//		return 0, errors.Wrap(err, "get VG used size")
//	}

//	used := 0

//	for i := range vgs {
//		used += vgs[i].Size
//	}

//	return used, nil
//}
