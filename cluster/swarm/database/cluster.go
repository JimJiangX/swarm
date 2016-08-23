package database

import (
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertClusterQuery = "INSERT INTO tb_cluster (id,name,type,storage_id,storage_type,networking_id,enabled,max_node,usage_limit) VALUES (:id,:name,:type,:storage_id,:storage_type,:networking_id,:enabled,:max_node,:usage_limit)"

// tb_cluster structure
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

// Insert insert a new record to tb_cluster.
func (c Cluster) Insert() error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	// insert into database
	_, err = db.NamedExec(insertClusterQuery, &c)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertClusterQuery, &c)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Cluster.Insert")
}

// UpdateStatus update tb_cluster.enabled by ID
func (c *Cluster) UpdateStatus(state bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_cluster SET enabled=? WHERE id=?"

	_, err = db.Exec(query, state, c.ID)
	if err == nil {
		c.Enabled = state

		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, state, c.ID)
	if err != nil {
		return errors.Wrap(err, "update Cluster.Enabled by ID:"+c.ID)
	}

	c.Enabled = state

	return nil
}

// UpdateParams updates MaxNode\UsageLimit
func (c Cluster) UpdateParams() error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_cluster SET max_node=:max_node,usage_limit=:usage_limit WHERE id=:id OR name=:name"

	_, err = db.NamedExec(query, &c)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(query, &c)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "update Cluster MaxNode or UsageLimit")
}

// DeleteCluster delete a record of tb_cluster by nameOrID
func DeleteCluster(nameOrID string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tb_cluster WHERE id=? OR name=?"

	_, err = db.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "delete Cluster:"+nameOrID)
}

// GetCluster get Cluster by nameOrID.
func GetCluster(nameOrID string) (Cluster, error) {
	c := Cluster{}

	db, err := GetDB(false)
	if err != nil {
		return c, err
	}

	const query = "SELECT * FROM tb_cluster WHERE id=? OR name=?"

	err = db.Get(&c, query, nameOrID, nameOrID)
	if err == nil {
		return c, nil
	}
	if _err := CheckError(err); _err == ErrNoRowsFound {
		return c, errors.Wrap(err, "not found Cluster:"+nameOrID)
	}

	db, err = GetDB(true)
	if err != nil {
		return c, err
	}

	err = db.Get(&c, query, nameOrID, nameOrID)
	if err == nil {
		return c, nil
	}

	return c, errors.Wrap(err, "get Cluster:"+nameOrID)

}

// ListClusters select tb_cluster
func ListClusters() ([]Cluster, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var clusters []Cluster
	const query = "SELECT * FROM tb_cluster"

	err = db.Select(&clusters, query)
	if err == nil {
		return clusters, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&clusters, query)
	if err == nil {
		return clusters, nil
	}

	return nil, errors.Wrap(err, "list Clusters")
}

// CountClusterByStorage count Clusters by storageID.
func CountClusterByStorage(storageID string) (int, error) {
	db, err := GetDB(false)
	if err != nil {
		return 0, err
	}

	count := 0
	const query = "SELECT COUNT(id) from tb_cluster WHERE storage_id=?"

	err = db.Get(&count, query, storageID)
	if err == nil {
		return count, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return 0, err
	}

	err = db.Get(&count, query, storageID)
	if err == nil {
		return count, err
	}

	return 0, errors.Wrap(err, "Count Cluster By storage_id:"+storageID)
}

const insertNodeQuery = "INSERT INTO tb_node (id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:engine_id,:room,:seat,:max_container,:status,:register_at,:deregister_at)"

// tb_node structure
type Node struct {
	ID           string `db:"id"`
	Name         string `db:"name"`
	ClusterID    string `db:"cluster_id"`
	Addr         string `db:"admin_ip"`
	EngineID     string `db:"engine_id"`
	Room         string `db:"room"`
	Seat         string `db:"seat"`
	MaxContainer int    `db:"max_container"`
	Status       int64  `db:"status"`

	RegisterAt   time.Time `db:"register_at"`
	DeregisterAt time.Time `db:"deregister_at"`
}

func (n Node) TableName() string {
	return "tb_node"
}

// TxInsertMultiNodeAndTask insert nodes and tasks in one Tx
func TxInsertMultiNodeAndTask(nodes []*Node, tasks []*Task) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// use prepare insert into database
	stmt, err := tx.PrepareNamed(insertNodeQuery)
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

// UpdateStatus returns error when Node UPDATE tb_node.status.
func (n *Node) UpdateStatus(state int64) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_node SET status=? WHERE id=?"

	_, err = db.Exec(query, state, n.ID)
	if err == nil {
		atomic.StoreInt64(&n.Status, state)

		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, state, n.ID)
	if err == nil {
		atomic.StoreInt64(&n.Status, state)

		return nil
	}

	return errors.Wrap(err, "update Node Status")
}

// UpdateParams returns error when Node update max_container.
func (n *Node) UpdateParams(max int) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_node SET max_container=? WHERE id=?"

	_, err = db.Exec(query, max, n.ID)
	if err == nil {
		n.MaxContainer = max

		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, max, n.ID)
	if err == nil {
		n.MaxContainer = max

		return nil
	}

	return errors.Wrap(err, "update Node MaxContainer by ID:"+n.ID)
}

// TxUpdateNodeStatus returns error when Node UPDATE status.
func TxUpdateNodeStatus(n *Node, task *Task, nstate, tstate int64, msg string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE tb_node SET status=? WHERE id=?", nstate, n.ID)
	if err != nil {
		return err
	}

	err = txUpdateTaskStatus(tx, task, tstate, time.Now(), msg)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "Tx update Node Status by ID:"+n.ID)
	}

	n.Status = nstate

	return nil
}

// TxUpdateNodeRegister returns error when Node UPDATE infomation.
func TxUpdateNodeRegister(n *Node, task *Task, nstate, tstate int64, eng, msg string) error {
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

	err = txUpdateTaskStatus(tx, task, tstate, time.Now(), msg)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "Tx update Node Status by ID:"+n.ID)
	}

	atomic.StoreInt64(&n.Status, nstate)

	return nil
}

// GetNode get Node by nameOrID.
func GetNode(nameOrID string) (Node, error) {
	db, err := GetDB(false)
	if err != nil {
		return Node{}, err
	}

	node := Node{}
	const query = "SELECT * FROM tb_node WHERE id=? OR name=? OR engine_id=?"

	err = db.Get(&node, query, nameOrID, nameOrID, nameOrID)
	if err == nil {
		return node, nil
	}
	if _err := CheckError(err); _err == ErrNoRowsFound {
		return node, errors.Wrap(err, "not found Node by:"+nameOrID)
	}

	db, err = GetDB(true)
	if err != nil {
		return Node{}, err
	}

	err = db.Get(&node, query, nameOrID, nameOrID, nameOrID)
	if err == nil {
		return node, nil
	}

	return node, errors.Wrap(err, "get Node by:"+nameOrID)
}

// GetNodeByAddr returns Node by addr.
func GetNodeByAddr(addr string) (Node, error) {
	db, err := GetDB(false)
	if err != nil {
		return Node{}, err
	}

	node := Node{}
	const query = "SELECT * FROM tb_node WHERE admin_ip=?"

	err = db.Get(&node, query, addr)
	if err == nil {
		return node, nil
	}
	if _err := CheckError(err); _err == ErrNoRowsFound {
		return node, errors.Wrap(err, "not found Node by addr:"+addr)
	}

	db, err = GetDB(true)
	if err != nil {
		return Node{}, err
	}

	err = db.Get(&node, query, addr)
	if err == nil {
		return node, nil
	}

	return node, errors.Wrap(err, "get Node by addr:"+addr)
}

// GetAllNodes returns all nodes.
func GetAllNodes() ([]Node, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	const query = "SELECT * FROM tb_node"

	err = db.Select(&nodes, query)
	if err == nil {
		return nodes, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&nodes, query)
	if err == nil {
		return nodes, nil
	}

	return nil, errors.Wrap(err, "get all Nodes")
}

// ListNodeByCluster returns nodes,select by cluster
func ListNodeByCluster(cluster string) ([]*Node, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	const query = "SELECT * FROM tb_node WHERE cluster_id=?"

	err = db.Select(&nodes, query, cluster)
	if err == nil {
		return nodes, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&nodes, query, cluster)
	if err == nil {
		return nodes, nil
	}

	return nil, errors.Wrap(err, "list Node by cluster:"+cluster)
}

// CountNodeByCluster returns num of node select by cluster.
func CountNodeByCluster(cluster string) (int, error) {
	db, err := GetDB(false)
	if err != nil {
		return 0, err
	}

	num := 0
	const query = "SELECT COUNT(id) FROM tb_node WHERE cluster_id=?"

	err = db.Get(&num, query, cluster)
	if err == nil {
		return num, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return 0, err
	}

	err = db.Get(&num, query, cluster)
	if err == nil {
		return num, nil
	}

	return 0, errors.Wrap(err, "count Node by cluster:"+cluster)
}

// ListNodesByEngines returns nodes,select by engines ID.
func ListNodesByEngines(names []string) ([]Node, error) {
	if len(names) == 0 {
		return []Node{}, nil
	}

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
	if err == nil {
		return nodes, nil
	}

	return nil, errors.Wrapf(err, "list Nodes by engines:%s", names)
}

// ListNodesByClusters returns nodes,select by clusters\type\enabled.
func ListNodesByClusters(clusters []string, _type string, enable bool) ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var clist []string
	err = db.Select(&clist, "SELECT id FROM tb_cluster WHERE type=? AND enabled=?", _type, enable)
	if err != nil {
		return nil, errors.Wrapf(err, "list Cluster by type='%s',enabled=%t", _type, enable)
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

	if len(list) == 0 && len(clist) > 0 {
		list = clist
	}
	if len(list) == 0 {
		return []Node{}, nil
	}

	query, args, err := sqlx.In("SELECT * FROM tb_node WHERE cluster_id IN (?);", list)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	err = db.Select(&nodes, query, args...)
	if err == nil {
		return nodes, nil
	}

	return nil, errors.Wrap(err, "list Nodes by clusters")
}

// DeleteNode delete node by name or ID
func DeleteNode(nameOrID string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tb_node WHERE id=? OR name=?"

	_, err = db.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "delete Node by nameOrID:"+nameOrID)
}
