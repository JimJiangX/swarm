package compose

type Clone struct {
}

func newCloneManager() Composer {
	return &Clone{}
}

func (c *Clone) ClearCluster() error {
	return nil
}

func (c *Clone) CheckCluster() error {
	return nil
}

func (c *Clone) ComposeCluster() error {
	return nil
}
