package deploy

import (
	"encoding/json"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/garden"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type serviceWithTask struct {
	spec structs.ServiceSpec
	task database.Task
}

// Deployment deploy containers
type Deployment struct {
	gd *garden.Garden
}

// New returns a pointer of Deployment
func New(gd *garden.Garden) *Deployment {
	return &Deployment{
		gd: gd,
	}
}

// Deploy build and run Service,task run in a goroutine.
func (d *Deployment) Deploy(ctx context.Context, spec structs.ServiceSpec, compose bool) (structs.PostServiceResponse, error) {
	resp := structs.PostServiceResponse{}
	auth, err := d.gd.AuthConfig()
	if err != nil {
		return resp, err
	}

	svc, task, err := d.gd.BuildService(spec)
	if err != nil {
		return resp, err
	}

	t, err := svc.Spec()
	if err != nil {
		return resp, err
	}

	resp.ID = t.ID
	resp.Name = t.Name
	resp.TaskID = task.ID
	resp.Units = make([]structs.UnitNameID, len(t.Units))
	for i := range t.Units {
		resp.Units[i] = structs.UnitNameID{
			ID:   t.Units[i].ID,
			Name: t.Units[i].Name,
		}
	}

	go d.deployV2(ctx, svc, compose, task, auth)

	return resp, nil
}

// DeployServices deploy slice of Service if service not exist,tasks run in goroutines.
func (d *Deployment) DeployServices(ctx context.Context, services []structs.ServiceSpec, compose bool) ([]structs.PostServiceResponse, error) {
	list, err := d.gd.ListServices(ctx)
	if err != nil && !database.IsNotFound(err) {
		return nil, err
	}

	existing := make(map[string]structs.ServiceSpec, len(list))
	for _, service := range list {
		existing[service.Name] = service
	}

	for i := range services {
		if _, exist := existing[services[i].Name]; exist {
			return nil, errors.Errorf("Duplicate entry '%s' for key 'Service.Name'", services[i].Name)
		}
	}

	auth, err := d.gd.AuthConfig()
	if err != nil {
		return nil, err
	}

	out := make([]structs.PostServiceResponse, 0, len(services))

	for _, spec := range services {

		service, task, err := d.gd.BuildService(spec)
		if err != nil {
			return out, err
		}

		spec, err := service.Spec()
		if err != nil {
			return out, err
		}

		out = append(out, structs.PostServiceResponse{
			ID:     spec.ID,
			Name:   spec.Name,
			TaskID: task.ID,
		})

		go d.deployV2(ctx, service, compose, task, auth)
	}

	return out, nil
}

func (d *Deployment) deploy(ctx context.Context, svc *garden.Service, compose bool,
	t *database.Task, auth *types.AuthConfig) (err error) {

	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("deploy:%v", r)
		}

		if err == nil {
			t.Status = database.TaskDoneStatus
		} else {
			t.Status = database.TaskFailedStatus
		}

		t.SetErrors(err)

		_err := d.gd.Ormer().SetTask(*t)

		logrus.WithField("Service", svc.ID()).Infof("deploy service,since=%s,%+v %+v", time.Since(start), _err, err)
	}()

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	actor := alloc.NewAllocator(d.gd.Ormer(), d.gd.Cluster)
	pendings, err := d.gd.Allocation(ctx, actor, svc)
	if err != nil {
		return err
	}

	err = svc.CreateContainer(ctx, pendings, auth)
	if err != nil {
		return err
	}

	err = svc.InitStart(ctx, "", d.gd.KVClient(), nil, nil, false, nil)
	if err != nil {
		return err
	}

	if compose {
		err = svc.Compose(ctx)
	}

	return err
}

func (d *Deployment) deployV2(ctx context.Context,
	svc *garden.Service, compose bool,
	task *database.Task, auth *types.AuthConfig) error {

	return d.gd.DeployService(ctx, svc, compose, task, auth)
}

// Link is exported,not done yet.
func (d *Deployment) Link(ctx context.Context, links structs.ServicesLink) (string, error) {
	start := time.Now()

	links, err := d.freshServicesLink(links)
	if err != nil {
		return "", err
	}

	task := database.NewTask("deploy link:"+links.Mode, database.ServiceLinkTask, "", "", nil, 300)
	err = d.gd.Ormer().InsertTask(task)
	if err != nil {
		return "", err
	}

	runLink := func() error {
		// generate new units config and commands,and sorted
		resp, err := d.gd.PluginClient().ServicesLink(ctx, links)
		if err != nil {
			return err
		}

		var (
			svc       *garden.Service
			serviceID string
		)

		// update units config file.
		for _, ul := range resp.Links {
			if ul.ServiceID == "" || ul.NameOrID == "" || ul.ConfigFile == "" {
				continue
			}

			if ul.ServiceID != serviceID {
				s := d.serviceFromLinks(links, ul.ServiceID)
				if s == nil {
					return errors.Errorf("not found Service '%s' from ServicesLink", ul.ServiceID)
				}

				svc = s
				serviceID = ul.ServiceID
			}

			err := svc.UpdateUnitConfig(ctx, ul.NameOrID, ul.ConfigFile, ul.ConfigContent)
			if err != nil {
				return err
			}
		}

		// start units service
		for _, ul := range resp.Links {
			if ul.ServiceID == "" || ul.NameOrID == "" || len(ul.Commands) == 0 {
				continue
			}

			if ul.ServiceID != serviceID {
				s := d.serviceFromLinks(links, ul.ServiceID)
				if s == nil {
					return errors.Errorf("not found Service '%s' from ServicesLink", ul.ServiceID)
				}

				svc = s
				serviceID = ul.ServiceID
			}

			err := svc.Exec(ctx, structs.ServiceExecConfig{
				Container: ul.NameOrID,
				Cmd:       ul.Commands,
			}, false, nil)

			if err != nil {
				return err
			}
		}

		// service compose
		for _, name := range resp.Compose {
			svc := d.serviceFromLinks(links, name)
			if svc == nil {
				return errors.Errorf("not found Service '%s' from ServicesLink", name)
			}

			err := svc.Compose(ctx)
			if err != nil {
				return err
			}
		}

		// service interval requests
		for _, ul := range resp.Links {
			if ul.Request == nil {
				continue
			}

			logrus.Debugf("LINK:%s %s\nBody:%s", ul.Request.Method, ul.Request.URL, ul.Request.Body)
		retry:
			for i := 3; i > 0; i-- {
				err = ul.Request.Send(ctx)
				if err == nil {
					break retry
				}
				time.Sleep(time.Second)
			}
			if err != nil {
				return err
			}
		}

		// reload service config
		for _, name := range resp.ReloadServicesConfig {
			svc := d.serviceFromLinks(links, name)
			if svc == nil {
				return errors.Errorf("not found Service '%s' from ServicesLink", name)
			}

			_, err := svc.ReloadServiceConfig(ctx, "")
			if err != nil {
				return err
			}
		}

		return nil
	}

	go func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf("deploy link,panic:%v", r)
			}

			if err == nil {
				task.Status = database.TaskDoneStatus
			} else {
				task.Status = database.TaskFailedStatus
			}

			task.SetErrors(err)

			_err := d.gd.Ormer().SetTask(task)

			logrus.Infof("deploy link %s,since=%s,%+v %+v", links.Mode, time.Since(start), _err, err)
		}()

		err = runLink()

		return err
	}()

	return task.ID, nil
}

func (d *Deployment) serviceFromLinks(links structs.ServicesLink, nameOrID string) *garden.Service {
	for _, l := range links.Links {
		if l.Spec != nil &&
			(l.Spec.ID == nameOrID || l.Spec.Name == nameOrID) {
			return d.gd.NewService(l.Spec, nil)
		}
	}

	return nil
}

func (d *Deployment) freshServicesLink(links structs.ServicesLink) (structs.ServicesLink, error) {
	ids := links.LinkIDs()

	switch len(ids) {
	case 0:
		return links, nil
	case 1:
		svc, err := d.gd.Service(ids[0])
		if err != nil {
			return links, err
		}

		spec, err := svc.Spec()
		if err != nil {
			return links, err
		}

		links.Links[0].Spec = spec

		return links, nil
	}

	out, err := d.gd.Ormer().ListServicesInfo()
	if err != nil {
		return links, err
	}

	m := make(map[string]database.ServiceInfo, len(ids))

	for i := range ids {
		for o := range out {
			if ids[i] == out[o].Service.ID || ids[i] == out[o].Service.Name {
				m[ids[i]] = out[o]
				break
			}
		}
	}

	containers := d.gd.Cluster.Containers()
	linkSlice := make([]*structs.ServiceLink, len(links.Links), len(ids))
	copy(linkSlice, links.Links)

	for l := range linkSlice {

		info, ok := m[linkSlice[l].ID]
		if ok {
			spec := garden.ConvertServiceInfo(d.gd.Cluster, info, containers)
			linkSlice[l].Spec = &spec
		}

		delete(m, linkSlice[l].ID)
	}

	for _, val := range m {
		spec := garden.ConvertServiceInfo(d.gd.Cluster, val, containers)
		linkSlice = append(linkSlice, &structs.ServiceLink{
			ID:   spec.ID,
			Spec: &spec,
		})
	}

	links.Links = linkSlice

	links.Sort()

	return links, nil
}

// ServiceScale scale service.
func (d *Deployment) ServiceScale(ctx context.Context, nameOrID string, scale structs.ServiceScaleRequest) (structs.ServiceScaleResponse, error) {
	svc, err := d.gd.Service(nameOrID)
	if err != nil {
		return structs.ServiceScaleResponse{}, err
	}

	actor := alloc.NewAllocator(d.gd.Ormer(), d.gd.Cluster)

	return d.gd.Scale(ctx, svc, actor, scale, true)
}

// ServiceUpdateImage update Service image version
func (d *Deployment) ServiceUpdateImage(ctx context.Context, name, version string, async bool) (string, error) {
	orm := d.gd.Ormer()

	im, err := orm.GetImageVersion(version)
	if err != nil {
		return "", err
	}

	svc, err := d.gd.Service(name)
	if err != nil {
		return "", err
	}

	im1, err := svc.Image()
	if err != nil {
		return "", err
	}

	if im1.Name != im.Name || im1.Major != im.Major {
		return "", errors.Errorf("Service:%s unsupported image update from %s to %s", name, im1.Image(), im.Image())
	}

	authConfig, err := d.gd.AuthConfig()
	if err != nil {
		return "", err
	}

	t := database.NewTask(svc.Name(), database.ServiceUpdateImageTask, svc.ID(), "", nil, 300)

	err = svc.UpdateImage(ctx, d.gd.KVClient(), im, &t, async, authConfig)

	return t.ID, err
}

// ServiceUpdate update service CPU & memory & volume resource,task run in goroutine.
func (d *Deployment) ServiceUpdate(ctx context.Context, name string, config structs.UpdateUnitRequire) (string, error) {
	svc, err := d.gd.Service(name)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	spec, err := svc.Spec()
	if err != nil {
		return "", err
	}

	task := database.NewTask(spec.Name, database.ServiceUpdateTask, spec.ID, string(out), nil, 300)
	err = d.gd.Ormer().InsertTask(task)
	if err != nil {
		return "", err
	}

	update := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf("deploy update:%v", r)
			}

			if err == nil {
				task.Status = database.TaskDoneStatus
			} else {
				task.Status = database.TaskFailedStatus

				logrus.Errorf("service update error %+v", err)
			}

			task.SetErrors(err)

			_err := d.gd.Ormer().SetTask(task)
			if _err != nil {
				logrus.Errorf("service update task error,%+v", _err)
			}
		}()

		actor := alloc.NewAllocator(d.gd.Ormer(), d.gd.Cluster)

		err = func() error {
			d.gd.Lock()
			defer d.gd.Unlock()

			return svc.UpdateResource(ctx, actor, config.Require.CPU, config.Require.Memory)
		}()
		if err != nil {
			return err
		}

		select {
		default:
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		}

		if len(config.Volumes) > 0 {
			err = func() error {
				d.gd.Lock()
				defer d.gd.Unlock()

				return svc.VolumeExpansion(actor, config.Volumes)
			}()
		}
		if err != nil {
			return err
		}

		select {
		default:
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		}

		if len(config.Networks) > 0 {
			err = func() error {
				d.gd.Lock()
				defer d.gd.Unlock()

				return svc.UpdateNetworking(ctx, actor, config.Networks)
			}()
		}

		return err
	}

	go update()

	return task.ID, err
}
