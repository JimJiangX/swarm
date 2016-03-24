package swarm

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
)

type Image struct {
	database.Software
	image *cluster.Image
}

func (r *Region) GetImage(name, version string) (Image, error) {
	sw, err := database.QueryImage(name, version)
	if err != nil {
		return Image{}, err
	}

	out := Image{Software: sw}

	image := r.Image(sw.ImageID)
	if image == nil {
		return out, nil
	}

	out.image = image

	return out, nil
}

func (r *Region) getImageByID(id string) (Image, error) {
	sw, err := database.QueryImageByID(id)
	if err != nil {
		return Image{}, err
	}

	out := Image{Software: sw}

	image := r.Image(out.ImageID)
	if image == nil {
		return out, nil
	}

	out.image = image

	return out, nil
}
