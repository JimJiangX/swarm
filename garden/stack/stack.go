package stack

import (
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

func (s *Stack) DeployServices() error {
	list, err := s.gd.ListServices()
	if err != nil && errors.Cause(err) != sql.ErrNoRows {
		return err
	}

	existingServiceMap := make(map[string]structs.ServiceSpec)
	for _, service := range list {
		existingServiceMap[service.Name] = service
	}

	sorted := servicesByPriority(s.services)
	sort.Sort(sorted)

	auth, err := s.gd.AuthConfig()
	if err != nil {
		return err
	}

	for _, spec := range sorted {
		if _, exist := existingServiceMap[spec.Name]; exist {
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

	return nil
}
