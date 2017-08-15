package compose

type mongodb struct {
}

func (mongodb) ClearCluster() error   { return nil }
func (mongodb) CheckCluster() error   { return nil }
func (mongodb) ComposeCluster() error { return nil }
