package driver

import (
	"bufio"
	"bytes"
	"io"
	"path/filepath"
	"strconv"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

func newNFSDriver(iface VolumeIface, engineID, sourceDir, backupDir string) (Driver, error) {
	n, err := iface.GetNode(engineID)
	if err != nil {
		return nil, err
	}

	if n.NFS.Addr == "" {
		return nil, nil
	}

	abs, err := utils.GetAbsolutePath(true, sourceDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return NewNFSDriver(n.NFS, filepath.Dir(abs), backupDir), nil
}

type _NFSDriver struct {
	vgIface
	database.NFS
	backupDir string
	baseDir   string
}

// NewNFSDriver returns _NFS Driver
func NewNFSDriver(nfs database.NFS, base, backup string) Driver {
	return _NFSDriver{
		vgIface:   unsupportSAN{},
		NFS:       nfs,
		backupDir: backup,
		baseDir:   base,
	}
}

func (nd _NFSDriver) Driver() string { return "NFS" }
func (nd _NFSDriver) Name() string   { return "" }
func (nd _NFSDriver) Type() string   { return "NFS" }

func (nd _NFSDriver) Space() (Space, error) {
	out, err := execNFScmd(nd.baseDir, nd.Addr, nd.Dir, nd.MountDir, nd.Options)
	if err != nil {
		return Space{}, err
	}

	total, free, err := parseNFSSpace(out)
	if err != nil {
		return Space{}, err
	}

	return Space{
		Total: total,
		Free:  free,
	}, nil
}

func (nd _NFSDriver) Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error) {
	if req.Type == "NFS" || req.Type == "nfs" {
		config.HostConfig.Binds = append(config.HostConfig.Binds, nd.MountDir+":"+nd.backupDir)
	}

	return nil, nil
}

func (nd _NFSDriver) Expand(_ string, size int64) error {
	return nil
}

func (nd _NFSDriver) Recycle(database.Volume) error {
	return nil
}

func parseNFSSpace(in []byte) (int64, int64, error) {

	atoi := func(line, key []byte) (int64, error) {

		if i := bytes.Index(line, key); i != -1 {
			return strconv.ParseInt(string(bytes.TrimSpace(line[i+len(key):])), 10, 64)
		}

		return 0, errors.Errorf("key:%s not exist", key)
	}

	var total, free int64
	tkey := []byte("total_space:")
	fkey := []byte("free_space:")

	br := bufio.NewReader(bytes.NewReader(in))

	for {
		if total > 0 && free > 0 {
			return total, free, nil
		}

		line, _, err := br.ReadLine()
		if err != nil {
			if err == io.EOF {
				return total, free, nil
			}

			return total, free, errors.Wrapf(err, "parse nfs output error,input:'%s'", in)
		}

		n, err := atoi(line, tkey)
		if err == nil {
			total = n
			continue
		}

		n, err = atoi(line, fkey)
		if err == nil {
			free = n
		}
	}
}

func execNFScmd(base, ip, dir, mount, opts string) ([]byte, error) {
	p := []string{base, "nfs", "get_NFS_space.sh"}

	path, err := utils.GetAbsolutePath(false, p...)
	if err != nil {
		return nil, errors.Errorf("nfs cmd error,%s,%s", filepath.Join(p...), err)
	}

	cmd := utils.ExecScript(path, ip, dir, mount, opts)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, errors.WithStack(err)
	}

	return out, nil
}
