package swarm

import (
	"fmt"

	"github.com/docker/swarm/cluster/swarm/store"
)

func (gd Gardener) GetStore(id string) (store.Store, error) {
	gd.RLock()

	for i := range gd.stores {
		if gd.stores[i].ID() == id {
			gd.RUnlock()
			return gd.stores[i], nil
		}
	}

	gd.RUnlock()

	store, err := store.GetStoreByID(id)
	if err == nil && store != nil {
		gd.Lock()
		gd.stores = append(gd.stores, store)
		gd.Unlock()

		return store, nil
	}

	return nil, fmt.Errorf("Storage Not Found,%s", id)
}

func (gd *Gardener) AddStore(store store.Store) error {
	s, err := gd.GetStore(store.ID())
	if err == nil && s != nil {
		return fmt.Errorf("Store is exist,%s", store.ID())
	}

	gd.Lock()
	gd.stores = append(gd.stores, store)
	gd.Unlock()

	return nil
}

func (gd *Gardener) AddStoreSpace(id string, space int) error {
	s, err := gd.GetStore(id)
	if err != nil {
		return err
	}

	_, err = s.AddSpace(space)

	return err
}
