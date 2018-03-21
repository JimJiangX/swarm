package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/scplib"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/plugin/parser/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// LoadImage load a new Image
func LoadImage(ctx context.Context,
	cluster cluster.Cluster,
	ormer database.ImageOrmer,
	pc api.PluginAPI,
	req structs.PostLoadImageRequest,
	timeout time.Duration) (string, string, error) {

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

	image := database.Image{
		ID:       utils.Generate32UUID(),
		Name:     req.Name,
		Major:    req.Major,
		Minor:    req.Minor,
		Patch:    req.Patch,
		Dev:      req.Dev,
		Labels:   labels,
		UploadAt: time.Now(),
	}

	task := database.NewTask(req.Image(), database.ImageLoadTask, image.ID, "load image", nil, int(timeout/time.Second))

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
			ch <- loadImage(ctx, cluster, ormer, pc, image, task, path, timeout)
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

	tl := tasklock.NewGoTask(database.ImageLoadTask, image.ID, &task, before, after)

	err = tl.Go(func(int) bool { return true }, run)
	if err != nil {
		return "", "", err
	}

	return image.ID, task.ID, nil
}

func loadImage(ctx context.Context,
	cluster cluster.Cluster,
	ormer database.ImageOrmer,
	pc api.PluginAPI,
	image database.Image,
	task database.Task,
	path string, timeout time.Duration) error {

	sys, err := ormer.GetSysConfig()
	if err != nil {
		return err
	}

	oldName := image.Image()
	newName := fmt.Sprintf("%s:%d/%s", sys.Registry.Domain, sys.Registry.Port, oldName)
	script := fmt.Sprintf("docker load -i %s && docker tag %s %s && docker push %s", path, oldName, newName, newName)

	field := logrus.WithField("Image", oldName)
	field.Infof("ssh exec:'%s'", script)

	addr := fmt.Sprintf("%s:%d", sys.Registry.Address, sys.Registry.SSHPort)
	scp, err := scplib.NewClientByPublicKeys(addr, sys.Registry.OsUsername, "", time.Duration(timeout)*time.Second)
	if err != nil {
		field.Errorf("load image,'%s@%s',exec:'%s'", sys.Registry.OsUsername, addr, script)
		return err
	}
	defer scp.Close()

	out, err := scp.Exec(script)
	if err != nil {
		field.Errorf("load image,exec:'%s',output:%s", script, out)
		return err
	}

	imageID, size, err := parsePushImageOutput(out)
	if err != nil {
		field.Errorf("parse output:%s", out)
		return err
	}
	{
		// update image & task
		image.ImageID = imageID
		image.Size = size
		image.UploadAt = time.Now()

		task.FinishedAt = image.UploadAt
		task.Status = database.TaskDoneStatus
		task.SetErrors(nil)

		err = ormer.SetImageAndTask(image, task)
		if err != nil {
			return err
		}
	}
	{
		// post image template to plugin
		err := postImageTemplate(ctx, pc, oldName, path)
		if err != nil {
			field.Errorf("post image config template,%+v", err)
		}
	}
	{
		// sync image to all engines
		go syncEnginesImages(cluster.ListEngines(), []string{newName}, sys.AuthConfig())
	}

	return nil
}

func parsePushImageOutput(in []byte) (string, int, error) {
	index := bytes.Index(in, []byte("digest:"))
	if index == -1 {
		return "", 0, errors.New("not found ImageID:" + string(in))
	}

	parts := bytes.Split(in[index:], []byte{' '})

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

// post image template to plugin
func postImageTemplate(ctx context.Context, pc api.PluginAPI, image, path string) error {
	tmpl, err := readImageTemplateFile(path)
	if err != nil {
		return err
	}

	if tmpl.Image != image {
		tmpl.Image = image
	}

	return pc.PostImageTemplate(ctx, tmpl)
}

func readImageTemplateFile(path string) (ct structs.ConfigTemplate, err error) {
	ext := filepath.Ext(path)
	index := 1 + len(path) - len(ext)

	{
		// read ConfigTemplate,xxxxx.json
		name := path[:index] + "json"

		dat, err := ioutil.ReadFile(name)
		if err != nil {
			return ct, errors.Wrap(err, name)
		}

		err = json.Unmarshal(dat, &ct)
		if err != nil {
			return ct, errors.Wrap(err, name)
		}
	}
	{
		// read template content,xxxxx.tmpl
		name := path[:index] + "tmpl"

		content, err := ioutil.ReadFile(name)
		if err != nil {
			return ct, errors.Wrap(err, name)
		}

		ct.Content = string(content)
	}

	if ct.Timestamp == 0 {
		ct.Timestamp = time.Now().Unix()
	}

	return ct, nil
}

func walkPath(path, ext string) string {
	e := filepath.Ext(path)

	return path[:1+len(path)-len(e)] + ext
}

// SyncEnginesImages all engines sync images
func SyncEnginesImages(engines []*cluster.Engine, images []string, ormer database.GetSysConfigIface) error {
	sys, err := ormer.GetSysConfig()
	if err != nil {
		return err
	}

	for i := range images {
		images[i] = fmt.Sprintf("%s:%d/%s", sys.Registry.Domain, sys.Registry.Port, images[i])
	}

	syncEnginesImages(engines, images, sys.AuthConfig())

	return nil
}

func syncEnginesImages(engines []*cluster.Engine, images []string, auth *types.AuthConfig) {
	ch := make(chan struct{}, 5)

	for i := range engines {
		ch <- struct{}{}

		go func(eng *cluster.Engine) {

			for _, image := range images {

				err := eng.Pull(image, auth, nil)
				if err != nil {
					logrus.Warnf("Engine %s pull image %s failed,%s", eng.Addr, image, err)
				}
			}

			<-ch
		}(engines[i])
	}
}

type lister interface {
	ListImages() ([]database.Image, error)
}

func syncEngineImages(eng *cluster.Engine, lister lister, sys database.SysConfig) error {
	list, err := lister.ListImages()
	if err != nil {
		return err
	}

	images := make([]string, 0, len(list))
	for i := range list {
		if list[i].Name == "" {
			continue
		}
		images = append(images, fmt.Sprintf("%s:%d/%s", sys.Registry.Domain, sys.Registry.Port, list[i].Image()))
	}

	syncEnginesImages([]*cluster.Engine{eng}, images, sys.AuthConfig())

	return nil
}
