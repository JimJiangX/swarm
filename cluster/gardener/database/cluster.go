package database

import "time"

type Cluster struct {
	ID          string  `db:"id"`
	Name        string  `db:"name"`
	Type        string  `db:"type"`
	StorageType string  `db:"storage_type"`
	StorageID   string  `db:"storage_id"`
	Datacenter  string  `db:"datacenter"`
	Enabled     bool    `db:"enabled"`
	MaxNode     int     `db:"max_node"`
	UsageLimit  float32 `db:"usage_limit"`
}

func (c Cluster) TableName() string {
	return "tb_cluster"
}

func (c *Cluster) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	// insert into database
	query := "INSERT INTO tb_cluster (id,name,type,storage_id,storage_type,datacenter,enabled,max_node,usage_limit) VALUES (:id,:name,:type,:storage_id,:storage_type,:datacenter,:enabled,:max_node,:usage_limit)"
	_, err = db.Exec(query, c)

	return err
}

func (c *Cluster) UpdateStatus(state bool) error {

	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_cluster SET enabled=? WHERE id=?", state, c.ID)
	if err != nil {
		return err
	}

	c.Enabled = state

	return nil
}

func GetCluster(id string) (*Cluster, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	c := &Cluster{}
	err = db.Get(c, "SELECT * FROM tb_cluster WHERE id=?", id)

	return c, err
}

type Node struct {
	ID        string `db:"id"`
	Name      string `db:"name"`
	ClusterID string `db:"cluster_id"`
	Addr      string `db:"admin_ip"`
	/*
		NCPU            int   `db:"ncpu"`
		MemoryByte      int64 `db:"mem_total"`
		LocalDataVGByte int64 `db:"local_data_vg"`
		StorageHostID   int   `db:"storage_host_id"`

		Architecture    string `db:"arch"`
		OperatingSystem string `db:"os"`
		KernelVersion   string `db:"kernel_version"`

		AdminNIC    string `db:"admin_NIC"`
		InternalNIC string `db:"internal_NIC"`
		ExternalNIC string `db:"external_NIC"`
		HBAWWW      string `db:"HBA_www"`

		DockerID       string `db:"docker_id"`
		DockertVersion string `db:"docker_version"`
		APIVersion     string `db:"api_version"`
		Labels         string `db:"labels"`
	*/
	MaxContainer int  `db:"max_container"`
	Status       byte `db:"status"`

	RegisterAt   time.Time `db:"register_at"`
	DeregisterAt time.Time `db:"deregister_at"`
}

func (n Node) TableName() string {
	return "tb_node"
}

func (n *Node) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	// insert into database
	query := "INSERT INTO tb_node (id,name,cluster_id,addr,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:addr,:max_container,:status,:register_at,:deregister_at)"
	_, err = db.Exec(query, n)

	return err
}

func GetNode(id string) (Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return Node{}, err
	}

	n := Node{}
	err = db.QueryRowx("SELECT * FROM tb_node WHERE id=?", id).StructScan(&n)

	return n, err
}

// UpdateStatus returns error when Node UPDATE status.
func (n *Node) UpdateStatus(state byte) error {

	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_node SET status=? WHERE id=?", state, n.ID)
	if err != nil {
		return err
	}

	n.Status = state

	return nil
}

func ListNode() ([]*Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	query := "SELECT * FROM tb_node WHERE status>1"

	rows, err := db.QueryRowx(query).SliceScan()
	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, 0, len(rows))

	for i := range rows {
		if node, ok := rows[i].(*Node); ok {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}
