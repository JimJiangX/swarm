package database

import (
	"fmt"
	"time"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
)

type Cluster struct {
	ID           string  `db:"id"`
	Name         string  `db:"name"`
	Type         string  `db:"type"`
	StorageType  string  `db:"storage_type"`
	StorageID    string  `db:"storage_id"`
	NetworkingID string  `db:"networking_id"`
	Enabled      bool    `db:"enabled"`
	MaxNode      int     `db:"max_node"`
	UsageLimit   float32 `db:"usage_limit"`
}

func (c Cluster) TableName() string {
	return "tb_cluster"
}

func NewCluster(name, _type, storageType, storageID, networking string,
	enable bool, num int, limit float32) Cluster {
	return Cluster{
		ID:           utils.Generate64UUID(),
		Name:         name,
		Type:         _type,
		StorageType:  storageType,
		StorageID:    storageID,
		NetworkingID: networking,
		Enabled:      enable,
		MaxNode:      num,
		UsageLimit:   limit,
	}
}

func (c Cluster) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	// insert into database
	query := "INSERT INTO tb_cluster (id,name,type,storage_id,storage_type,networking_id,enabled,max_node,usage_limit) VALUES (:id,:name,:type,:storage_id,:storage_type,:networking_id,:enabled,:max_node,:usage_limit)"
	_, err = db.NamedExec(query, &c)

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

// UpdateParams Updates MaxNode\UsageLimit
func (c Cluster) UpdateParams() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_cluster SET max_node=:max_node,usage_limit=:usage_limit WHERE id=:id OR name=:name"
	_, err = db.NamedExec(query, &c)

	return err
}

func DeleteCluster(IDOrName string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_cluster WHERE id=? OR name=?", IDOrName, IDOrName)

	return err
}

func GetCluster(IDOrName string) (Cluster, error) {
	db, err := GetDB(true)
	if err != nil {
		return Cluster{}, err
	}

	c := Cluster{}
	err = db.Get(&c, "SELECT * FROM tb_cluster WHERE id=? OR name=?", IDOrName, IDOrName)

	return c, err
}

func ListCluster() ([]Cluster, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	clusters := make([]Cluster, 0, 10)
	err = db.Select(&clusters, "SELECT * FROM tb_cluster")
	if err != nil {
		return nil, err
	}

	return clusters, nil
}

func CountClusterByStorage(storage string) (int, error) {
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	count := 0
	err = db.Get(&count, "SELECT COUNT(*) from tb_cluster WHERE storage_id=?", storage)

	return count, err
}

type Node struct {
	ID           string `db:"id"`
	Name         string `db:"name"`
	ClusterID    string `db:"cluster_id"`
	Addr         string `db:"admin_ip"`
	EngineID     string `db:"engine_id"`
	Room         string `db:"room"`
	Seat         string `db:"seat"`
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

func (n Node) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	// insert into database
	query := "INSERT INTO tb_node (id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:engine_id,:room,:seat,:max_container,:status,:register_at,:deregister_at)"
	_, err = db.NamedExec(query, &n)

	return err
}

func TxInsertNodeAndTask(node Node, task Task) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// insert into database
	query := "INSERT INTO tb_node (id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:engine_id,:room,:seat,:max_container,:status,:register_at,:deregister_at)"
	_, err = tx.NamedExec(query, &node)
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
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// insert into database
	query := "INSERT INTO tb_node (id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:engine_id,:room,:seat,:max_container,:status,:register_at,:deregister_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}
	for i := range nodes {
		_, err = stmt.Exec(nodes[i])
		if err != nil {
			stmt.Close()
			return err
		}
	}
	stmt.Close()

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

	node := Node{}
	err = db.Get(&node, "SELECT * FROM tb_node WHERE id=? OR name=? OR engine_id=?", IDOrName, IDOrName, IDOrName)

	return node, err
}

func GetAllNodes() ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	nodes := make([]Node, 0, 50)
	err = db.Select(&nodes, "SELECT * FROM tb_node")
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

// UpdateParams returns error when Node UPDATE max_container.
func (n *Node) UpdateParams(max int) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_node SET max_container=? WHERE id=?", max, n.ID)
	if err != nil {
		return err
	}

	n.MaxContainer = max

	return nil
}

// TxUpdateNodeStatus returns error when Node UPDATE status.
func TxUpdateNodeStatus(n *Node, task *Task, nstate, tstate int, msg string) error {
	tx, err := GetTX()
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
	tx, err := GetTX()
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

func ListNode(status int) ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	nodes := make([]Node, 0, 50)
	err = db.Select(&nodes, "SELECT * FROM tb_node WHERE status=?", status)
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

	nodes := make([]*Node, 0, 50)
	err = db.Select(&nodes, "SELECT * FROM tb_node WHERE cluster_id=?", cluster)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func CountNodeByCluster(cluster string) (int, error) {
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	num := 0
	err = db.Get(&num, "SELECT COUNT(*) FROM tb_node WHERE cluster_id=?", cluster)
	if err != nil {
		return 0, err
	}

	return num, nil
}

func ListNodeByClusterType(_type string, enabled bool) ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var clist []string
	err = db.Select(&clist, "SELECT id FROM tb_cluster WHERE type=? AND enabled=?", _type, enabled)
	if err != nil {
		return nil, err
	}

	if len(clist) <= 0 {
		return nil, fmt.Errorf("No Available Cluster,Type='%s' & ebabled=%t", _type, enabled)
	}

	query, args, err := sqlx.In("SELECT * FROM tb_node WHERE cluster_id IN (?);", clist)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	err = db.Select(&nodes, query, args...)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func ListNodesByEngines(names []string) ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In("SELECT * FROM tb_node WHERE engine_id IN (?);", names)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	err = db.Select(&nodes, query, args...)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func ListEnginesByNodes(names []string) ([]string, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In("SELECT engine_id FROM tb_node WHERE id IN (?);", names)
	if err != nil {
		return nil, err
	}

	var nodes []string
	err = db.Select(&nodes, query, args...)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func ListNodesByClusters(clusters []string, _type string) ([]Node, error) {
	if len(clusters) == 0 {
		return nil, nil
	}

	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var clist []string
	err = db.Select(&clist, "SELECT id FROM tb_cluster WHERE type=? AND enabled=?", _type, true)
	if err != nil {
		return nil, err
	}

	list := make([]string, 0, len(clusters))
	for i := range clusters {
		for j := range clist {
			if clusters[i] == clist[j] {
				list = append(list, clusters[i])
				break
			}
		}
	}

	query, args, err := sqlx.In("SELECT * FROM tb_node WHERE cluster_id IN (?);", list)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	err = db.Select(&nodes, query, args...)
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
