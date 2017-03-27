package deploy

import (
	"encoding/json"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/garden"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type serviceWithTask struct {
	spec structs.ServiceSpec
	task database.Task
}

type Deployment struct {
	gd *garden.Garden
}

func New(gd *garden.Garden) *Deployment {
	return &Deployment{
		gd: gd,
	}
}

func (d *Deployment) Deploy(ctx context.Context, spec structs.ServiceSpec) (structs.PostServiceResponse, error) {
	resp := structs.PostServiceResponse{}
	auth, err := d.gd.AuthConfig()
	if err != nil {
		return resp, err
	}

	svc, task, err := d.gd.BuildService(spec)
	if err != nil {
		return resp, err
	}

	resp.ID = svc.Spec().ID
	resp.Name = svc.Spec().Name
	resp.TaskID = task.ID

	go d.deploy(ctx, svc, task, auth)

	return resp, nil
}

func (d *Deployment) DeployServices(ctx context.Context, services []structs.ServiceSpec) ([]structs.PostServiceResponse, error) {
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

		out = append(out, structs.PostServiceResponse{
			ID:     service.Spec().ID,
			Name:   service.Spec().Name,
			TaskID: task.ID,
		})

		go d.deploy(ctx, service, task, auth)
	}

	return out, nil
}

func (d *Deployment) deploy(ctx context.Context, svc *garden.Service, t *database.Task, auth *types.AuthConfig) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}

		if err == nil {
			t.Errors = ""
			t.Status = database.TaskDoneStatus
		} else {
			t.Errors = err.Error()
			t.Status = database.TaskFailedStatus

			logrus.WithField("Service", svc.Spec().Name).Errorf("service deploy error %+v", err)
		}

		_err := d.gd.Ormer().SetTask(*t)
		if _err != nil {
			logrus.WithField("Service", svc.Spec().Name).Errorf("deploy task error,%+v", _err)
		}
	}()

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	actor := resource.NewAllocator(d.gd.Ormer(), d.gd.Cluster)
	pendings, err := d.gd.Allocation(ctx, actor, svc)
	if err != nil {
		return err
	}

	err = svc.RunContainer(ctx, pendings, auth)
	if err != nil {
		return err
	}

	kvc := d.gd.KVClient()

	err = svc.InitStart(ctx, kvc, nil, svc.Spec().Options)
	if err != nil {
		return err
	}

	pc := d.gd.PluginClient()

	err = pc.ServiceCompose(ctx, svc.Spec())

	return err
}

func (d *Deployment) Link(ctx context.Context, links []*structs.ServiceLink) (string, error) {
	err := d.freshServicesLink(links)
	if err != nil {
		return "", err
	}

	// TODO:better task info
	task := database.NewTask("stack link", database.ServiceLinkTask, "", "", "", 300)

	go func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf("stack link,panic:%v", r)
			}
			if err == nil {
				task.Errors = ""
				task.Status = database.TaskDoneStatus
			} else {
				task.Errors = err.Error()
				task.Status = database.TaskFailedStatus

				logrus.Errorf("stack link and start,%+v", err)
			}

			_err := d.gd.Ormer().SetTask(task)
			if _err != nil {
				logrus.Errorf("stack link and start,%+v", _err)
			}
		}()

		err = d.gd.PluginClient().ServicesLink(ctx, links)

		return err
	}()

	return task.ID, nil
}

func (d *Deployment) freshServicesLink(links structs.ServicesLink) error {
	ids := links.Links()

	switch len(ids) {
	case 0:
		return nil
	case 1:
		svc, err := d.gd.Service(ids[0])
		if err != nil {
			return err
		}

		spec := svc.Spec()
		links[0].Spec = &spec

		return nil
	}

	out, err := d.gd.Ormer().ListServicesInfo()
	if err != nil {
		return err
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

	links.Sort()

	for l := range links {

		info, ok := m[links[l].ID]
		if ok {
			spec := garden.ConvertServiceInfo(info)
			links[l].Spec = &spec
		}

		delete(m, links[l].ID)
	}

	for _, val := range m {
		spec := garden.ConvertServiceInfo(val)
		links = append(links, &structs.ServiceLink{
			ID:   spec.ID,
			Spec: &spec,
		})
	}

	return nil
}

func (d *Deployment) ServiceScale(ctx context.Context, nameOrID string, arch structs.Arch) (string, error) {
	orm := d.gd.Ormer()

	table, err := orm.GetService(nameOrID)
	if err != nil {
		return "", err
	}

	units, err := orm.ListUnitByServiceID(table.ID)
	if err != nil {
		return "", err
	}

	if len(units) == arch.Replicas {
		return "", nil
	}

	svc, err := d.gd.GetService(table.ID)
	if err != nil {
		return "", err
	}

	// spec := svc.Spec()
	// task := database.NewTask(spec.Name, database.ServiceScaleTask, spec.ID, fmt.Sprintf("replicas=%d", replicas), "", 300)

	if len(units) > arch.Replicas {
		err = svc.ScaleDown(ctx, d.gd.KVClient(), arch.Replicas)
	}
	if err != nil {
		return "", err
	}

	{
		desc := *table.Desc
		desc.ID = utils.Generate32UUID()

		out, err := json.Marshal(arch)
		if err == nil {
			table.DescID = desc.ID
			table.Desc = &desc
			desc.Replicas = arch.Replicas
			desc.Architecture = string(out)
		}

		err = orm.SetServiceDesc(table)

		return "", err
	}
}

func (d *Deployment) ServiceUpdateImage(ctx context.Context, name, version string) (string, error) {
	orm := d.gd.Ormer()

	im, err := orm.GetImageVersion(version)
	if err != nil {
		return "", err
	}

	svc, err := d.gd.GetService(name)
	if err != nil {
		return "", err
	}

	im1, err := svc.Image()
	if err != nil {
		return "", err
	}

	if im1.Name != im.Name || im1.Major != im.Major {
		return "", errors.Errorf("Service:%s unsupported image update from %s to %s", name, im1.Version(), im.Version())
	}

	authConfig, err := d.gd.AuthConfig()
	if err != nil {
		return "", err
	}

	t := database.NewTask(svc.Spec().Name, database.ServiceUpdateImageTask, svc.Spec().ID, "", "", 300)

	err = svc.UpdateImage(ctx, d.gd.KVClient(), im, t, authConfig)
	if err != nil {
		return t.ID, err
	}

	return t.ID, err
}

func (d *Deployment) ServiceUpdate(ctx context.Context, name string, config structs.UnitRequire) (string, error) {
	table, err := d.gd.Ormer().GetService(name)
	if err != nil {
		return "", err
	}

	svc, err := d.gd.GetService(table.ID)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	t := database.NewTask(svc.Spec().Name, database.ServiceUpdateTask, svc.Spec().ID, string(out), "", 300)
	actor := resource.NewAllocator(d.gd.Ormer(), d.gd.Cluster)

	if (config.Require.CPU > 0 && table.Desc.NCPU != config.Require.CPU) ||
		(config.Require.Memory > 0 && table.Desc.Memory != config.Require.Memory) {

		ncpu := config.Require.CPU
		if ncpu == 0 {
			ncpu = table.Desc.NCPU
		}
		memory := config.Require.Memory
		if memory == 0 {
			memory = table.Desc.Memory
		}

		err = func() error {
			d.gd.Lock()
			defer d.gd.Unlock()

			return svc.ServiceUpdate(ctx, actor, int64(ncpu), memory, t)
		}()

		desc := *table.Desc
		desc.ID = utils.Generate32UUID()
		desc.NCPU = ncpu
		desc.Memory = memory

		table.DescID = desc.ID
		table.Desc = &desc
	}

	err = d.gd.Ormer().SetServiceDesc(table)

	return t.ID, err
}