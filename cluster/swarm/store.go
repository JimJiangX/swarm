package swarm

import (
	"fmt"

	"github.com/docker/swarm/cluster/swarm/store"
)

func (r Region) GetStore(id string) (store.Store, error) {
	r.RLock()

	for i := range r.stores {
		if r.stores[i].ID() == id {
			r.RUnlock()
			return r.stores[i], nil
		}
	}

	r.RUnlock()

	return nil, fmt.Errorf("Store Not Found,%s", id)
}

func (r *Region) AddStore(store store.Store) error {
	s, err := r.GetStore(store.ID())
	if err == nil && s != nil {
		return fmt.Errorf("Store is exist,%s", store.ID())
	}

	r.Lock()
	r.stores = append(r.stores, store)
	r.Unlock()

	return nil
}

func (r *Region) AddStoreSpace(id string, space int) error {
	s, err := r.GetStore(id)
	if err != nil {
		return err
	}

	_, err = s.AddSpace(space)

	return err
}
