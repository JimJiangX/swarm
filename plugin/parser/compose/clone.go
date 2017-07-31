package compose

type clone struct {
	scriptDir string
}

func newCloneManager(dir string) Composer {
	return &clone{scriptDir: dir}
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
