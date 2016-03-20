package gardener

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/gardener/database"
	"github.com/samalba/dockerclient"
)

func (r *Region) GetImage(IDOrName, version string) (*cluster.Image, error) {
	image := r.Image(IDOrName)
	if image != nil {
		return image, nil
	}
	if version != "" {
		image = r.Image(IDOrName + version)
		if image != nil {
			return image, nil
		}
	}

	sw, err := database.QueryImage(IDOrName, version)
	if err != nil {
		return &cluster.Image{Image: dockerclient.Image{
			Id: sw.ID}}, nil
	}

	return nil, err
}
