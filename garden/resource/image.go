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
func LoadImage(ctx context.Context, ormer database.ImageOrmer, req structs.PostLoadImageRequest) (string, string, error) {
	path, err := utils.GetAbsolutePath(false, req.Path)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	req.Path = path

	var labels string
	if len(req.Labels) > 0 {
		buf := bytes.NewBuffer(nil)
		err := json.NewEncoder(buf).Encode(req.Labels)
		if err != nil {
			return "", "", errors.Wrapf(err, "parse Labels:%s", req.Labels)
		}
		labels = buf.String()
	}

	registry, err := ormer.GetRegistry()
	if err != nil {
		return "", "", err
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
	task := database.NewTask(req.Version(), database.ImageLoadTask, image.ID, "load image", nil, 300)

	err = ormer.InsertImageWithTask(image, task)
	if err != nil {
		return "", "", err
	}

	go func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("load image panic::%+v", r)
			}

			if err != nil {
				logrus.WithField("Image", req.Name).Errorf("load image:%+v", err)

				task.FinishedAt = time.Now()
				task.Status = database.TaskFailedStatus
				task.SetErrors(err)

				err := ormer.SetTask(task)
				if err != nil {
					logrus.WithField("Image", req.Name).Errorf("update image task:%+v", err)
				}
			}
		}()

		oldName := req.Version()
		newName := fmt.Sprintf("%s:%d/%s", registry.Domain, registry.Port, oldName)
		script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", req.Path, oldName, newName, newName)

		scp, err := scplib.NewScpClient(registry.Address, registry.OsUsername, registry.OsPassword)
		if err != nil {
			logrus.WithField("Image", req.Name).Errorf("load image,'%s@%s',exec:'%s'", registry.OsUsername, registry.Address, script)
			return err
		}
		defer scp.Close()

		out, err := scp.Exec(script)
		if err != nil {
			logrus.WithField("Image", req.Name).Errorf("load image,exec:'%s',output:%s", script, out)
			return err
		}

		imageID, size, err := parsePushImageOutput(out)
		if err != nil {
			logrus.WithField("Image", req.Name).Errorf("parse output:%s", out)
			return err
		}

		image.ImageID = imageID
		image.Size = size
		image.UploadAt = time.Now()

		task.FinishedAt = image.UploadAt
		task.Status = database.TaskDoneStatus
		task.SetErrors(nil)

		err = ormer.SetImageAndTask(image, task)

		return err
	}()

	return image.ID, task.ID, err
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
