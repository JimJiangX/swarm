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
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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

	return err
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

func LoadImage(req structs.PostLoadImageRequest) (string, string, error) {
	content, err := ioutil.ReadFile(req.ConfigFilePath)
	if err != nil {
		err = fmt.Errorf("ReadAll From ConfigFile %s error:%s", req.ConfigFilePath, err)
		logrus.Error(err)

		return "", "", err
	}
	parser, _, err := initialize(req.Name)
	if err != nil {
		return "", "", err
	}

	_, err = parser.ParseData(content)
	if err != nil {
		return "", "", fmt.Errorf("Parse PostLoadImageRequest.Content Error:%s", err)
	}

	config, err := database.GetSystemConfig()
	if err != nil {
		return "", "", err
	}

	_imageID := utils.Generate64UUID()

	background := func(ctx context.Context) error {
		buffer := bytes.NewBuffer(nil)
		oldName := fmt.Sprintf("%s:%s", req.Name, req.Version)
		newName := fmt.Sprintf("%s:%d/%s", config.Registry.Domain, config.Registry.Port, oldName)
		script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", req.Path, oldName, newName, newName)

		err = SSHCommand(config.Registry.Address,
			config.Registry.OsUsername, config.Registry.OsPassword, script, buffer)
		if err != nil {
			logrus.Error(err, buffer.String())
			return err
		}

		imageID, size, err := parsePushImageOutput(buffer.String())
		if err != nil {
			return err
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
			ID:               _imageID,
			Name:             req.Name,
			Version:          req.Version,
			ImageID:          imageID,
			Labels:           buf.String(),
			Size:             size,
			TemplateConfigID: unitConfig.ID,
			UploadAt:         time.Now(),
		}
		unitConfig.ImageID = image.ID

		err = database.TxInsertImage(image, unitConfig)
		if err != nil {
			logrus.Error(err)
		}

		return err
	}

	task := database.NewTask("load image", _imageID, "", nil, 0)
	t := NewAsyncTask(context.Background(), background, task.Insert, task.UpdateStatus, 0)

	return _imageID, task.ID, t.Run()
}

func UpdateImageTemplateConfig(imageID string, req structs.UpdateUnitConfigRequest) (database.UnitConfig, error) {
	image, config, err := database.GetImageAndUnitConfig(imageID)
	if err != nil {
		return config, err
	}

	newConfig := config
	newConfig.ID = utils.Generate64UUID()
	newConfig.Version = 0

	if len(req.ConfigKVs) > 0 {
		parser, _, err := initialize(image.Name)
		if err != nil {
			return config, err
		}

		configer, err := parser.ParseData(nil)
		if err != nil {
			return config, errors.Wrap(err, "ParseDate")
		}

		keysets := make(map[string]database.KeysetParams, len(config.KeySets))
		for _, kv := range req.ConfigKVs {
			err := configer.Set(kv.Key, kv.Value)
			if err != nil {
				return config, errors.Wrap(err, "Configer Set")
			}
			keysets[kv.Key] = database.KeysetParams{
				Key:         kv.Key,
				CanSet:      kv.CanSet,
				MustRestart: kv.MustRestart,
				Description: kv.Description,
			}
		}
		newConfig.KeySets = keysets

		content, err := parser.Marshal()
		if err != nil {
			return config, errors.Wrap(err, "Marshal")
		}
		newConfig.Content = string(content)
	}

	if req.ConfigMountPath != "" {
		newConfig.Mount = req.ConfigMountPath
	}

	err = database.TxUpdateImageTemplateConfig(image.ID, newConfig)
	if err != nil {
		return newConfig, err
	}

	return newConfig, nil
}

func parsePushImageOutput(in string) (string, int, error) {
	index := strings.Index(in, "digest:")
	if index == -1 {
		return "", 0, fmt.Errorf("Not Found ImageID,%s", in)
	}

	output := in[index:]

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
		logrus.Errorf("Not Found Image %s:%s,Error:%s", name, version, err)

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
