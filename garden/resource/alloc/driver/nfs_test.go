package driver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/swarm/garden/database"
)

func TestParseNFSSpace(t *testing.T) {
	output := `
	2017/04/19 16:36:34 NFS
	total_space:45145088
free_space:   37890048
free_space:3789079799
`

	total, free, err := parseNFSSpace([]byte(output))
	if err != nil {
		t.Errorf("%+v", err)
	}

	if total != 45145088 || free != 37890048 {
		t.Errorf("expected (%d,%d),but got (%d,%d)", 45145088, 37890048, total, free)
	}
}

func TestNFSSpace(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	base := filepath.Join(gopath, "src/github.com/docker/swarm/script")

	nd := NewNFSDriver(database.NFS{
		Addr:     "192.168.4.129",
		Dir:      "/NFSbackup",
		MountDir: "/NFSbackup",
		Options:  "nolock",
	}, base, "/BACKUP")

	_, err := nd.Space()
	if err != nil {
		t.Skip(err)
	}
}
