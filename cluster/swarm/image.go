package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
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

func (gd *Gardener) GetImageByID(id string) (Image, error) {
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

func (gd *Gardener) UpdateImageStatus(id string, enable bool) error {

	return database.UpdateImageStatus(id, enable)
}

func (gd *Gardener) RemoveImage(id string) error {
	err := database.DeleteImage(id)
	if err != nil {
		return err
	}

	_, err = gd.RemoveImages(id, false)

	return nil
}

func (gd *Gardener) LoadImage(req structs.PostLoadImageRequest) (string, error) {
	parser, _, err := initialize(req.Name)
	if err != nil {
		return "", err
	}

	_, err = parser.ParseData([]byte(req.Content))
	if err != nil {
		return "", fmt.Errorf("Parse PostLoadImageRequest.Content Error:%s", err.Error())
	}

	config, err := database.GetSystemConfig()
	if err != nil {
		return "", err
	}

	buffer := bytes.NewBuffer(nil)
	oldName := fmt.Sprintf("%s:%s", req.Name, req.Version)
	newName := fmt.Sprintf("%s:%d/%s", config.Registry.Domain, config.Registry.Port, oldName)
	script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", req.Path, oldName, newName, newName)

	err = SSHCommand(config.Registry.Address,
		config.Registry.OsUsername, config.Registry.OsPassword, script, buffer)
	if err != nil {
		return buffer.String(), err
	}

	imageID, size, err := parsePushImageOutput(buffer.String())
	if err != nil {
		return imageID, err
	}

	unitConfig := database.UnitConfig{
		ID:        utils.Generate64UUID(),
		ImageID:   imageID,
		Path:      req.ConfigPath,
		Version:   0,
		ParentID:  "",
		Content:   req.Content,
		KeySets:   req.KeySet,
		CreatedAt: time.Now(),
	}

	buf := bytes.NewBuffer(nil)
	json.NewEncoder(buf).Encode(req.Labels)

	image := database.Image{
		Enabled:          true,
		ID:               utils.Generate64UUID(),
		Name:             req.Name,
		Version:          req.Version,
		ImageID:          imageID,
		Labels:           buf.String(),
		Size:             size,
		TemplateConfigID: unitConfig.ID,
		UploadAt:         time.Now(),
	}

	task := database.NewTask("load image", image.ID, "", nil, 0)

	err = database.TxInsertImage(image, unitConfig, task)
	if err != nil {
		return imageID, err
	}

	return imageID, nil
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

func (gd *Gardener) GetImageName(id, name, version string) (string, string, error) {
	var (
		image Image
		err   error
	)
	// query image from database
	if id != "" {
		image, err = gd.GetImageByID(id)
	}

	if (err != nil || image.ID == "") && name != "" {
		image, err = gd.GetImage(name, version)
	}

	if err != nil {
		logrus.Errorf("Not Found Image %s:%s,Error:%s", name, version, err.Error())

		return "", "", err
	}

	if !image.Enabled {
		logrus.Errorf("Image %s is Disabled", image.ImageID)
		return "", "", err
	}

	config, err := database.GetSystemConfig()
	if err != nil {
		return "", "", err
	}

	imageName := fmt.Sprintf("%s:%d/%s:%s", config.Registry.Domain, config.Registry.Port, image.Name, image.Version)

	return imageName, image.ImageID, nil
}
