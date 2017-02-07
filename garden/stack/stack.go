package stack

import (
	"context"
	"database/sql"
	"sort"

	"github.com/docker/swarm/garden"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

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
	gd       *garden.Garden
	services []structs.ServiceSpec
}

func NewStack(gd *garden.Garden, services []structs.ServiceSpec) *Stack {
	return &Stack{
		gd:       gd,
		services: services,
	}
}

func (s *Stack) DeployServices(ctx context.Context) error {
	list, err := s.gd.ListServices(ctx)
	if err != nil && errors.Cause(err) != sql.ErrNoRows {
		return err
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
		return err
	}

	for _, spec := range sorted {
		if _, exist := existing[spec.Name]; exist {
			continue
		}

		service, err := s.gd.BuildService(spec)
		if err != nil {
			return err
		}

		pendings, err := s.gd.Allocation(service)
		if err != nil {
			return err
		}

		err = service.CreateContainer(pendings, auth)
		if err != nil {
			return err
		}
	}

	err = s.freshServices(ctx)
	if err != nil {
		return err
	}

	for i := range s.services {
		svc := s.gd.NewService(s.services[i])

		if _, ok := existing[s.services[i].Name]; ok {
			err = svc.Start(ctx)
		} else {
			if kvc := s.gd.KVClient(); kvc != nil {
				err = svc.InitStart(ctx, kvc, nil)
			}
			//TODO: KV Client is nil
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
