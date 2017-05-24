package compose

type clone struct {
}

func newCloneManager() Composer {
	return &clone{}
}

func (c *clone) ClearCluster() error {
	return nil
}

func (c *clone) CheckCluster() error {
	return nil
}

func (c *clone) ComposeCluster() error {
	return nil
}
