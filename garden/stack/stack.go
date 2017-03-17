package stack

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/garden"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type serviceWithTask struct {
	spec structs.ServiceSpec
	task database.Task
}

type Stack struct {
	gd *garden.Garden
}

func New(gd *garden.Garden) *Stack {
	return &Stack{
		gd: gd,
	}
}

func (s *Stack) Deploy(ctx context.Context, spec structs.ServiceSpec) (structs.PostServiceResponse, error) {
	resp := structs.PostServiceResponse{}
	auth, err := s.gd.AuthConfig()
	if err != nil {
		return resp, err
	}

	svc, task, err := s.gd.BuildService(spec)
	if err != nil {
		return resp, err
	}

	resp.ID = svc.Spec().ID
	resp.Name = svc.Spec().Name
	resp.TaskID = task.ID

	go s.deploy(ctx, svc, *task, auth)

	return resp, nil
}

func (s *Stack) DeployServices(ctx context.Context, services []structs.ServiceSpec) ([]structs.PostServiceResponse, error) {
	list, err := s.gd.ListServices(ctx)
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

	auth, err := s.gd.AuthConfig()
	if err != nil {
		return nil, err
	}

	out := make([]structs.PostServiceResponse, 0, len(services))

	for _, spec := range services {

		service, task, err := s.gd.BuildService(spec)
		if err != nil {
			return out, err
		}

		out = append(out, structs.PostServiceResponse{
			ID:     service.Spec().ID,
			Name:   service.Spec().Name,
			TaskID: task.ID,
		})

		go s.deploy(ctx, service, *task, auth)
	}

	return out, nil
}

func (s *Stack) deploy(ctx context.Context, svc *garden.Service, t database.Task, auth *types.AuthConfig) (err error) {
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

		_err := s.gd.Ormer().SetTask(t)
		if _err != nil {
			logrus.WithField("Service", svc.Spec().Name).Errorf("deploy task error,%+v", _err)
		}
	}()

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	pendings, err := s.gd.Allocation(ctx, svc)
	if err != nil {
		return err
	}

	err = svc.RunContainer(ctx, pendings, auth)
	if err != nil {
		return err
	}

	kvc := s.gd.KVClient()

	err = svc.InitStart(ctx, kvc, nil, svc.Spec().Options)
	if err != nil {
		return err
	}

	pc := s.gd.PluginClient()

	err = pc.ServiceCompose(ctx, svc.Spec())

	return err
}

func (s *Stack) LinkAndStart(ctx context.Context, links []*structs.ServiceLink) (string, error) {
	services, err := s.freshServices(links)
	if err != nil {
		return "", err
	}

	task := database.Task{}

	go func(s *Stack, services []structs.ServiceSpec, task database.Task) (err error) {
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

			_err := s.gd.Ormer().SetTask(task)
			if _err != nil {
				logrus.Errorf("stack link and start,%+v", _err)
			}
		}()

		kvc := s.gd.KVClient()

		for i := range services {
			svc := s.gd.NewService(services[i])

			err = svc.InitStart(ctx, kvc, nil, svc.Spec().Options)
			if err != nil {
				return err
			}
		}

		return nil

	}(s, services, task)

	return task.ID, nil
}

func (s *Stack) freshServices(links structs.ServicesLink) ([]structs.ServiceSpec, error) {
	ids := links.Links()

	switch len(ids) {
	case 0:
		return nil, nil
	case 1:

	default:
	}

	links.Sort()

	return nil, nil
}
