package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

type Image struct {
	database.Image
	image *cluster.Image
}

func (gd *Gardener) GetImage(name, version string) (Image, error) {
	im, err := database.QueryImage(name, version)
	if err != nil {
		return Image{}, err
	}

	out := Image{Image: im}

	image := gd.Image(im.ImageID)
	if image == nil {
		return out, nil
	}

	out.image = image

	return out, nil
}

func (gd *Gardener) getImageByID(id string) (Image, error) {
	im, err := database.QueryImageByID(id)
	if err != nil {
		return Image{}, err
	}

	out := Image{Image: im}

	image := gd.Image(out.ImageID)
	if image == nil {
		return out, nil
	}

	out.image = image

	return out, nil
}

func (gd *Gardener) LoadImage(req structs.PostLoadImageRequest) (string, error) {
	config, err := database.GetSystemConfig()
	if err != nil {
		return "", err
	}

	buffer := bytes.NewBuffer(nil)

	oldName := fmt.Sprintf("%s:%s", req.Name, req.Version)
	newName := fmt.Sprintf("%s:%d/%s", config.Registry.Domain, config.Registry.Port, oldName)
	script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", req.Path, oldName, newName, newName)

	err = SSHCommand(config.Registry.Address, strconv.Itoa(config.Registry.Port),
		config.Registry.OsUsername, config.Registry.OsPassword, script, buffer)
	if err != nil {
		return buffer.String(), err
	}

	ImageID, size, err := parsePushImageOutput(buffer.String())
	if err != nil {
		return ImageID, err
	}

	unitConfig := database.UnitConfig{
		ID:       utils.Generate64UUID(),
		ImageID:  ImageID,
		Path:     req.ConfigPath,
		Version:  0,
		ParentID: "",
		Content:  req.Content,
		KeySets:  req.KeySet,
		CreateAt: time.Now(),
	}

	buf := bytes.NewBuffer(nil)
	json.NewEncoder(buf).Encode(req.Labels)

	image := database.Image{
		Enabled:          true,
		ID:               utils.Generate64UUID(),
		Name:             req.Name,
		Version:          req.Version,
		ImageID:          ImageID,
		Labels:           buf.String(),
		Size:             size,
		TemplateConfigID: unitConfig.ID,
		UploadAt:         time.Now(),
	}

	task := database.NewTask("load image", image.ID, "", nil, 0)

	err = database.TxInsertImage(image, unitConfig, task)
	if err != nil {
		return ImageID, err
	}

	return ImageID, nil
}

func parsePushImageOutput(in string) (string, int, error) {
	index := strings.Index(in, "digest:")
	if index == -1 {
		return "", 0, fmt.Errorf("Not Found ImageID,%s", in)
	}

	output := string(in[index:])

	parts := strings.Split(output, " ")

	if len(parts) == 4 && parts[2] == "size:" {
		id := strings.TrimSpace(parts[1])

		size, err := strconv.Atoi(strings.TrimSpace(parts[3]))
		if err != nil {
			return id, size, fmt.Errorf("Parse Size Error,%s", parts[3])
		}

		return id, size, nil
	}

	return "", 0, fmt.Errorf("Parse Output Error,%s", in)

}
