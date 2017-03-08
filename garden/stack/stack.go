package stack

import (
	"database/sql"
	"sort"
	"sync"

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

type servicesByPriority []structs.ServiceSpec

func (sp servicesByPriority) Less(i, j int) bool {
	return sp[i].Priority > sp[j].Priority
}

// Len is the number of elements in the collection.
func (sp servicesByPriority) Len() int {
	return len(sp)
}

// Swap swaps the elements with indexes i and j.
func (sp servicesByPriority) Swap(i, j int) {
	sp[i], sp[j] = sp[j], sp[i]
}

func initServicesPriority(services []structs.ServiceSpec) servicesByPriority {
	priority := make(map[string]int, len(services))

	for i := range services {
		max := 0
		for _, d := range services[i].Deps {
			if d != nil {
				if d.Priority > 0 {
					priority[d.Name] = d.Priority
				}
				if priority[d.Name] > max {
					max = priority[d.Name]
				}
			}
		}

		if len(services[i].Deps) > 0 {
			priority[services[i].Name] = max + 1
		}
	}

	for i := range services {
		services[i].Priority = priority[services[i].Name]
	}

	return servicesByPriority(services)
}

type Stack struct {
	wg       *sync.WaitGroup
	gd       *garden.Garden
	services []structs.ServiceSpec
}

func New(gd *garden.Garden, services []structs.ServiceSpec) *Stack {
	return &Stack{
		wg:       new(sync.WaitGroup),
		gd:       gd,
		services: services,
	}
}

func (s *Stack) DeployServices(ctx context.Context) ([]structs.PostServiceResponse, error) {
	list, err := s.gd.ListServices(ctx)
	if err != nil && errors.Cause(err) != sql.ErrNoRows {
		return nil, err
	}

	existing := make(map[string]structs.ServiceSpec, len(list))
	for _, service := range list {
		existing[service.Name] = service
	}

	sorted := initServicesPriority(s.services)
	sort.Sort(sorted)

	s.services = sorted

	auth, err := s.gd.AuthConfig()
	if err != nil {
		return nil, err
	}

	out := make([]structs.PostServiceResponse, 0, len(s.services))

	for _, spec := range sorted {
		if _, exist := existing[spec.Name]; exist {
			continue
		}

		service, task, err := s.gd.BuildService(spec)
		if err != nil {
			return out, err
		}

		out = append(out, structs.PostServiceResponse{
			ID:     service.Spec().ID,
			Name:   service.Spec().Name,
			TaskID: task.ID,
		})

		s.wg.Add(1)

		go s.deploy(service, *task, auth)
	}

	go s.linkAndStart(ctx, existing)

	return out, nil
}

func (s *Stack) deploy(service *garden.Service, t database.Task, auth *types.AuthConfig) (err error) {
	defer func() {
		s.wg.Done()

		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}

		if err == nil {
			t.Errors = ""
			t.Status = database.TaskDoneStatus
		} else {
			t.Errors = err.Error()
			t.Status = database.TaskFailedStatus
		}

		_err := s.gd.Ormer().SetTask(t)
		if _err != nil {
			logrus.WithField("Service", service.Spec().Name).Errorf("%+v,error=%+v", _err, err)
		}
	}()

	pendings, err := s.gd.Allocation(service)
	if err != nil {
		logrus.WithField("Service", service.Spec().Name).Errorf("Service allocation error %+v", err)
		return err
	}

	err = service.CreateContainer(pendings, auth)
	if err != nil {
		logrus.WithField("Service", service.Spec().Name).Errorf("Service create containers error %+v", err)
	}

	return err

}

func (s *Stack) linkAndStart(ctx context.Context, existing map[string]structs.ServiceSpec) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("stack run,panic:%v", r)
			return
		}

		if err != nil {
			logrus.Errorf("stack run,%+v", err)
		}
	}()

	s.wg.Wait()

	kvc := s.gd.KVClient()

	err = s.freshServices(ctx)
	if err != nil {
		return err
	}

	for i := range s.services {
		svc := s.gd.NewService(s.services[i])

		if _, ok := existing[s.services[i].Name]; ok {

			err = svc.UpdateUnitsConfigs(ctx, nil)

		} else {

			err = svc.InitStart(ctx, kvc, nil)

		}

		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Stack) freshServices(ctx context.Context) error {
	list, err := s.gd.ListServices(ctx)
	if err != nil && errors.Cause(err) != sql.ErrNoRows {
		return err
	}

	existing := make(map[string]structs.ServiceSpec, len(list))
	for i := range list {
		existing[list[i].Name] = list[i]
	}

	sorted := initServicesPriority(s.services)
	sort.Sort(sorted)

	for i := range sorted {
		deps := make([]*structs.ServiceSpec, 0, len(sorted[i].Deps))
		for _, d := range sorted[i].Deps {
			if d == nil {
				continue
			}
			if spec, ok := existing[d.Name]; ok {
				deps = append(deps, &spec)
			} else {
				deps = append(deps, d)
			}
		}

		if spec, ok := existing[sorted[i].Name]; ok {
			sorted[i] = spec
			sorted[i].Deps = deps
		}
	}

	s.services = sorted

	return nil
}
