package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/scplib"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// LoadImage load a new Image
func LoadImage(ctx context.Context, ormer database.ImageOrmer, req structs.PostLoadImageRequest) (string, error) {
	var labels string
	if len(req.Labels) > 0 {
		buf := bytes.NewBuffer(nil)
		err := json.NewEncoder(buf).Encode(req.Labels)
		if err != nil {
			return "", errors.Wrapf(err, "parse Labels:%s", req.Labels)
		}
		labels = buf.String()
	}

	registry, err := ormer.GetRegistry()
	if err != nil {
		return "", err
	}

	image := database.Image{
		ID:       utils.Generate32UUID(),
		Name:     req.Name,
		Major:    req.Major,
		Minor:    req.Minor,
		Patch:    req.Patch,
		Labels:   labels,
		UploadAt: time.Now(),
	}

	err = ormer.InsertImage(image)
	if err != nil {
		return "", err
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.WithField("Image", req.Name).Errorf("load image panic:%+v", err)
			}
		}()

		oldName := req.Version()
		newName := fmt.Sprintf("%s:%d/%s", registry.Domain, registry.Port, oldName)
		script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", req.Path, oldName, newName, newName)

		scp, err := scplib.NewScpClient(registry.Address, registry.OsUsername, registry.OsPassword)
		if err != nil {
			logrus.WithField("Image", req.Name).Errorf("Load image,%+v", err)
			return
		}
		defer scp.Close()

		out, err := scp.Exec(script)
		if err != nil {
			logrus.WithField("Image", req.Name).Errorf("load image,%+v,output:%s", err, out)
			return
		}

		imageID, size, err := parsePushImageOutput(out)
		if err != nil {
			logrus.WithField("Image", req.Name).Errorf("parse output,%+v", err)
			return
		}

		err = ormer.SetImage(image.ID, imageID, size)
		if err != nil {
			logrus.WithField("Image", req.Name).Errorf("set image table,%+v", err)
		}
	}()

	return image.ID, err
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
