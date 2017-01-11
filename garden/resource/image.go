package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/scplib"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/plugin/client"
	"github.com/docker/swarm/plugin/parser/api"
	"github.com/pkg/errors"
)

// LoadImage load a new Image
func LoadImage(ctx context.Context, ormer database.Ormer, c client.Client, req structs.PostLoadImageRequest) (string, error) {
	text, err := ioutil.ReadFile(req.ConfigFilePath)
	if err != nil {
		return "", errors.Wrap(err, "ReadAll from configFile:"+req.ConfigFilePath)
	}

	pc := api.NewPlugin(c)
	template := structs.ConfigTemplate{
		Name:    req.Name,
		Version: req.Version,
		Mount:   req.ConfigMountPath,
		Content: text,
		Keysets: req.KeySets,
	}
	err = pc.ImageCheck(ctx, template)
	if err != nil {
		return "", err
	}

	registry, err := ormer.GetRegistry()
	if err != nil {
		return "", err
	}

	oldName := fmt.Sprintf("%s:%s", req.Name, req.Version)
	newName := fmt.Sprintf("%s:%d/%s", registry.Domain, registry.Port, oldName)
	script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", req.Path, oldName, newName, newName)

	scp, err := scplib.NewScpClient(registry.Address, registry.OsUsername, registry.OsPassword)
	if err != nil {
		return "", err
	}
	defer scp.Close()

	out, err := scp.Exec(script)
	if err != nil {
		logrus.Error(err, string(out))
		return "", err
	}

	imageID, size, err := parsePushImageOutput(out)
	if err != nil {
		return imageID, err
	}

	template.Image = imageID
	template.Timestamp = time.Now().Unix()

	pc.PostImageTemplate(ctx, template)
	if err != nil {
		return imageID, err
	}

	var labels string
	if len(req.Labels) > 0 {
		buf := bytes.NewBuffer(nil)
		json.NewEncoder(buf).Encode(req.Labels)
		labels = buf.String()
	}

	image := database.Image{
		Enabled:  true,
		ID:       imageID,
		Name:     req.Name,
		Version:  req.Version,
		Labels:   labels,
		Size:     size,
		UploadAt: time.Now(),
	}

	err = ormer.InsertImage(image)
	if err != nil {
		logrus.Error(err)
	}

	return imageID, err
}

func parsePushImageOutput(in []byte) (string, int, error) {
	index := bytes.Index(in, []byte("digest:"))
	if index == -1 {
		return "", 0, errors.New("not found ImageID:" + string(in))
	}

	output := in[index:]

	parts := bytes.Split(output, []byte{' '})

	if len(parts) == 4 && bytes.Equal(parts[2], []byte("size:")) {
		id := bytes.TrimSpace(parts[1])

		size, err := strconv.Atoi(string(bytes.TrimSpace(parts[3])))
		if err != nil {
			return string(id), size, errors.Wrap(err, "parse size:"+string(parts[3]))
		}

		return string(id), size, nil
	}

	return "", 0, errors.Errorf("parse output error:%s", in)
}
