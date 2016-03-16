package gardener

import (
	"fmt"
	"sync/atomic"
)

func (c *Cluster) ServiceToExecute(svc *Service) {
	c.serviceExecuteCh <- svc
}

func (c *Cluster) ServiceExecute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%s", r, err)
		}
	}()

	for {
		svc := <-c.serviceExecuteCh

		if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
			continue
		}

		atomic.StoreInt64(&svc.Status, 1)
	}

	return err
}
