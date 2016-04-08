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
	_, err = db.NamedExec(query, c)

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
	ID           string `db:"id"`
	Name         string `db:"name"`
	ClusterID    string `db:"cluster_id"`
	Addr         string `db:"admin_ip"`
	MaxContainer int    `db:"max_container"`
	Status       int    `db:"status"`

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
	query := "INSERT INTO tb_node (id,name,cluster_id,admin_ip,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:max_container,:status,:register_at,:deregister_at)"
	_, err = db.NamedExec(query, n)

	return err
}

func TxInsertNodeAndTask(node *Node, task *Task) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// insert into database
	query := "INSERT INTO tb_node (id,name,cluster_id,admin_ip,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:max_container,:status,:register_at,:deregister_at)"
	_, err = tx.NamedExec(query, node)
	if err != nil {
		return err
	}

	err = TxInsertTask(tx, task)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func TxInsertMultiNodeAndTask(nodes []*Node, tasks []*Task) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// insert into database
	query := "INSERT INTO tb_node (id,name,cluster_id,admin_ip,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:max_container,:status,:register_at,:deregister_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}
	for i := range nodes {
		_, err = stmt.Exec(nodes[i])
		if err != nil {
			return err
		}
	}

	err = TxInsertMultiTask(tx, tasks)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func GetNode(IDOrName string) (Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return Node{}, err
	}

	n := Node{}
	err = db.QueryRowx("SELECT * FROM tb_node WHERE id=? OR name=?", IDOrName, IDOrName).StructScan(&n)

	return n, err
}

// UpdateStatus returns error when Node UPDATE status.
func (n *Node) UpdateStatus(state int) error {
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

	var nodes []*Node
	query := "SELECT * FROM tb_node WHERE status>1"

	err = db.QueryRowx(query).StructScan(&nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func ListNodeByClusterType(tag string) ([]*Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	query := "SELECT * FROM tb_node WHERE type=? AND status>1"

	err = db.QueryRowx(query, tag).StructScan(&nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}
