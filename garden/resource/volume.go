package resource

func (n *Node) usedSize() (int, error) {
	lvs, err := n.vo.ListVolumeByNode(n.node.ID)
	if err != nil {
		return 0, err
	}

	used := 0
	for i := range lvs {
		used += lvs[i].Size
	}

	return used, nil
}

// total,free,used,error
func (n *Node) diskSize() (int, int, int, error) {
	// TODO: total,free
	used, err := n.usedSize()
	return 0, 0, used, err
}
