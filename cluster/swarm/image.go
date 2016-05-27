package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

func UpdateImageStatus(id string, enable bool) error {

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

// key is case sensitive,converte to lowcase
func converteToKeysetParams(params []structs.KeysetParams) map[string]database.KeysetParams {
	if len(params) == 0 {
		return nil
	}

	keyset := make(map[string]database.KeysetParams, len(params))
	for i := range params {
		key := strings.ToLower(params[i].Key) // case sensitive
		keyset[key] = database.KeysetParams{
			Key:         params[i].Key,
			CanSet:      params[i].CanSet,
			MustRestart: params[i].MustRestart,
			Description: params[i].Description,
		}
	}

	return keyset
}

func LoadImage(req structs.PostLoadImageRequest) (string, error) {
	content, err := ioutil.ReadFile(req.ConfigFilePath)
	if err != nil {
		err = fmt.Errorf("ReadAll From ConfigFile %s error:%s", req.ConfigFilePath, err)
		logrus.Error(err.Error())

		return "", err
	}
	parser, _, err := initialize(req.Name)
	if err != nil {
		return "", err
	}

	_, err = parser.ParseData(content)
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
		return "", err
	}

	unitConfig := database.UnitConfig{
		ID:        utils.Generate64UUID(),
		Mount:     req.ConfigMountPath,
		Version:   0,
		ParentID:  "",
		Content:   string(content),
		KeySets:   converteToKeysetParams(req.KeySet),
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
	unitConfig.ImageID = image.ID

	task := database.NewTask("load image", image.ID, "", nil, 0)

	err = database.TxInsertImage(image, unitConfig, task)
	if err != nil {
		return "", err
	}

	return image.ID, nil
}

func UpdateImageTemplateConfig(imageID string, req structs.UpdateUnitConfigRequest) (database.UnitConfig, error) {
	image, config, err := database.GetImageAndUnitConfig(imageID)
	if err != nil {
		return config, err
	}

	newConfig := config

	if req.ConfigFilePath != "" {
		content, err := ioutil.ReadFile(req.ConfigFilePath)
		if err != nil {
			err = fmt.Errorf("ReadAll From ConfigFile %s error:%s", req.ConfigFilePath, err)
			logrus.Error(err.Error())

			return config, err
		}
		if len(content) == 0 {
			return config, fmt.Errorf("read 0 byte in ConfigFile %s", req.ConfigFilePath)
		}
		parser, _, err := initialize(image.Name)
		if err != nil {
			return config, err
		}

		_, err = parser.ParseData(content)
		if err != nil {
			return config, fmt.Errorf("Parse PostLoadImageRequest.Content Error:%s", err.Error())
		}

		newConfig.Content = string(content)
	}

	if req.ConfigMountPath != "" {
		newConfig.Mount = req.ConfigMountPath
	}

	if len(req.KeySet) > 0 {
		newConfig.KeySets = converteToKeysetParams(req.KeySet)
	}

	err = database.UpdateUnitConfig(newConfig)
	if err != nil {
		return config, err
	}

	return newConfig, nil
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
