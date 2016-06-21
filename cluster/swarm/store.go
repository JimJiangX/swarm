package swarm

import (
	"fmt"

	"github.com/docker/swarm/cluster/swarm/database"
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

func (gd *Gardener) UpdateStoreSpaceStatus(id string, space int, state bool) error {
	storage, err := gd.GetStore(id)
	if err != nil {
		return err
	}

	if state {
		if err := storage.EnableSpace(space); err != nil {
			return err
		}
	} else {
		if err := storage.DisableSpace(space); err != nil {
			return err
		}
	}

	return nil
}

func (gd *Gardener) RemoveStoreSpace(id string, space int) error {
	_, err := gd.GetStore(id)
	if err != nil {
		return err
	}

	rg, err := database.GetRaidGroup(id, space)
	if err != nil {
		return err
	}

	count, err := database.CountLUNByRaidGroupID(rg.ID)
	if err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("Store %s RaidGroup %d is using,cannot Remove", id, space)
	}

	return database.DeleteRaidGroup(id, space)
}

func (gd *Gardener) RemoveStore(storage string) error {
	count, err := database.CountClusterByStorage(storage)
	if err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("Store %s is using,cannot be removed", storage)
	}

	err = database.DeleteStorageByID(storage)
	if err != nil {
		return err
	}

	gd.Lock()
	for i := range gd.stores {
		if gd.stores[i].ID() == storage {
			gd.stores = append(gd.stores[:i], gd.stores[i+1:]...)
			break
		}
	}
	gd.Unlock()

	return nil
}
