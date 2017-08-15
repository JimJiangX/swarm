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
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// LoadImage load a new Image
func LoadImage(ctx context.Context, ormer database.ImageOrmer, req structs.PostLoadImageRequest, timeout int) (string, string, error) {
	if timeout == 0 {
		timeout = 300
	}

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
		Build:    req.Build,
		Labels:   labels,
		UploadAt: time.Now(),
	}
	task := database.NewTask(req.Version(), database.ImageLoadTask, image.ID, "load image", nil, timeout)

	before := func(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error) {
		err = ormer.InsertImageWithTask(image, *t)
		if err != nil {
			return false, 0, err
		}

		return true, 0, nil
	}

	after := func(key string, val int, task *database.Task, t time.Time) error {
		if task == nil || task.Status == database.TaskDoneStatus {
			return nil
		}

		return ormer.SetTask(*task)
	}

	run := func() (err error) {
		ch := make(chan error)

		go func(ch chan<- error) {
			err := func() error {
				oldName := req.Version()
				newName := fmt.Sprintf("%s:%d/%s", registry.Domain, registry.Port, oldName)
				script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", req.Path, oldName, newName, newName)
				logrus.WithField("Image", req.Name).Infof("ssh exec:'%s'", script)

				scp, err := scplib.NewClientByPublicKeys(registry.Address, registry.OsUsername, "", time.Duration(timeout)*time.Second)
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

			ch <- err
		}(ch)

		select {
		case err = <-ch:
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		}

		return nil
	}

	tl := tasklock.NewGoTask(image.ID, &task, before, after)

	err = tl.Go(func(int) bool { return true }, run)
	if err != nil {
		return "", "", err
	}

	return image.ID, task.ID, nil
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
