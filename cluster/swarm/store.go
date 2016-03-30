package swarm

import (
	"fmt"

	"github.com/docker/swarm/cluster/swarm/store"
)

func (r Region) GetStore(id string) store.Store {
	r.RLock()

	for i := range r.stores {
		if r.stores[i].ID() == id {
			r.RUnlock()
			return r.stores[i]
		}
	}

	r.RUnlock()

	return nil
}

func (r *Region) AddStore(store store.Store) error {
	s := r.GetStore(store.ID())
	if s != nil {
		return fmt.Errorf("Store is exist,%s", store.ID())
	}

	r.Lock()
	r.stores = append(r.stores, store)
	r.Unlock()

	return nil
}
