package database

import (
	"time"

	"github.com/docker/swarm/utils"
)

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

func NewCluster(name, typ, storageType, storageID, dc string, enable bool, num int, limit float32) Cluster {
	return Cluster{
		ID:          utils.Generate64UUID(),
		Name:        name,
		Type:        typ,
		StorageType: storageType,
		StorageID:   storageID,
		Datacenter:  dc,
		Enabled:     enable,
		MaxNode:     num,
		UsageLimit:  limit,
	}
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

func DeleteCluster(IDOrName string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_cluster WHERE id=? OR name=?", IDOrName, IDOrName)

	return err
}

func GetCluster(IDOrName string) (*Cluster, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	c := &Cluster{}
	err = db.Get(c, "SELECT * FROM tb_cluster WHERE id=? OR name=?", IDOrName, IDOrName)
	if err != nil {
		return nil, err
	}

	return c, nil
}

type Node struct {
	ID           string `db:"id"`
	Name         string `db:"name"`
	ClusterID    string `db:"cluster_id"`
	Addr         string `db:"admin_ip"`
	EngineID     string `db:"engine_id"`
	MaxContainer int    `db:"max_container"`
	Status       int    `db:"status"`

	RegisterAt   time.Time `db:"register_at"`
	DeregisterAt time.Time `db:"deregister_at"`
}

func (n Node) TableName() string {
	return "tb_node"
}

func NewNode(name, clusterID, addr, eng string, num, status int, t1, t2 time.Time) Node {
	return Node{
		ID:           utils.Generate64UUID(),
		Name:         name,
		ClusterID:    clusterID,
		Addr:         addr,
		EngineID:     eng,
		MaxContainer: num,
		Status:       status,
		RegisterAt:   t1,
		DeregisterAt: t2,
	}
}

func (n *Node) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	// insert into database
	query := "INSERT INTO tb_node (id,name,cluster_id,admin_ip,engine_id,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:engine_id,:max_container,:status,:register_at,:deregister_at)"
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
	err = db.QueryRowx("SELECT * FROM tb_node WHERE id=? OR name=? OR engine_id=?", IDOrName, IDOrName, IDOrName).StructScan(&n)

	return n, err
}

func GetAllNodes() ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	err = db.QueryRowx("SELECT * FROM tb_node").StructScan(&nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
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

// TxUpdateNodeStatus returns error when Node UPDATE status.
func TxUpdateNodeStatus(n *Node, task *Task, nstate, tstate int, msg string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE tb_node SET status=? WHERE id=?", nstate, n.ID)
	if err != nil {
		return err
	}

	n.Status = nstate

	err = TxUpdateTaskStatus(tx, task, tstate, time.Now(), msg)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// TxUpdateNodeRegister returns error when Node UPDATE infomation.
func TxUpdateNodeRegister(n *Node, task *Task, nstate, tstate int, eng, msg string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if eng != "" {
		_, err = tx.Exec("UPDATE tb_node SET engine_id=?,status=?,register_at=? WHERE id=?", eng, nstate, time.Now(), n.ID)
	} else {
		_, err = tx.Exec("UPDATE tb_node SET status=?,register_at=? WHERE id=?", nstate, time.Now(), n.ID)
	}
	if err != nil {
		return err
	}

	n.Status = nstate

	err = TxUpdateTaskStatus(tx, task, tstate, time.Now(), msg)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func ListNode(status int) ([]*Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	query := "SELECT * FROM tb_node WHERE status=?"

	err = db.QueryRowx(query, status).StructScan(&nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func ListNodeByCluster(cluster string) ([]*Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	query := "SELECT * FROM tb_node WHERE cluster_id=?"

	err = db.QueryRowx(query, cluster).StructScan(&nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func DeleteNode(IDOrName string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_node WHERE id=? OR name=?", IDOrName, IDOrName)

	return err
}
