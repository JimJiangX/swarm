package database

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
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

func (c Cluster) Insert() error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	// insert into database
	query := "INSERT INTO tb_cluster (id,name,type,storage_id,storage_type,networking_id,enabled,max_node,usage_limit) VALUES (:id,:name,:type,:storage_id,:storage_type,:networking_id,:enabled,:max_node,:usage_limit)"
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

	return errors.Wrap(err, "Insert Cluster")
}

func (c *Cluster) UpdateStatus(state bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	query := "UPDATE tb_cluster SET enabled=? WHERE id=?"
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
		return errors.Wrap(err, "Update Cluster.Enabled By ID:"+c.ID)
	}

	c.Enabled = state

	return nil
}

// UpdateParams Updates MaxNode\UsageLimit
func (c Cluster) UpdateParams() error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	query := "UPDATE tb_cluster SET max_node=:max_node,usage_limit=:usage_limit WHERE id=:id OR name=:name"
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

	return errors.Wrap(err, "Update Cluster Params")
}

func DeleteCluster(IDOrName string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	query := "DELETE FROM tb_cluster WHERE id=? OR name=?"
	_, err = db.Exec(query, IDOrName, IDOrName)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, IDOrName, IDOrName)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete Cluster:"+IDOrName)
}

func GetCluster(nameOrID string) (Cluster, error) {
	c := Cluster{}

	db, err := GetDB(false)
	if err != nil {
		return c, err
	}

	query := "SELECT * FROM tb_cluster WHERE id=? OR name=?"
	err = db.Get(&c, query, nameOrID, nameOrID)
	if err == nil {
		return c, nil
	}
	if _err := CheckError(err); _err == ErrNoRowsFound {
		return c, errors.Wrap(err, "Not Found Cluster:"+nameOrID)
	}

	db, err = GetDB(true)
	if err != nil {
		return c, err
	}

	err = db.Get(&c, query, nameOrID, nameOrID)
	if err == nil {
		return c, nil
	}

	return c, errors.Wrap(err, "Not Found Cluster:"+nameOrID)

}

func ListClusters() ([]Cluster, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var clusters []Cluster
	query := "SELECT * FROM tb_cluster"

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

	return nil, errors.Wrap(err, "List Clusters")

}

func CountClusterByStorage(storageID string) (int, error) {
	db, err := GetDB(false)
	if err != nil {
		return 0, err
	}

	count := 0
	query := "SELECT COUNT(*) from tb_cluster WHERE storage_id=?"

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

// UpdateStatus returns error when Node UPDATE status.
func (n *Node) UpdateStatus(state int) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	query := "UPDATE tb_node SET status=? WHERE id=?"
	_, err = db.Exec(query, state, n.ID)
	if err == nil {
		n.Status = state

		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, state, n.ID)
	if err == nil {
		n.Status = state

		return nil
	}

	return errors.Wrap(err, "Update Node Status")
}

// UpdateParams returns error when Node UPDATE max_container.
func (n *Node) UpdateParams(max int) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	query := "UPDATE tb_node SET max_container=? WHERE id=?"
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

	return errors.Wrap(err, "Update Node Param By ID:"+n.ID)
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

	err = TxUpdateTaskStatus(tx, task, tstate, time.Now(), msg)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "TX Update Node Status "+n.ID)
	}

	n.Status = nstate

	return nil
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

	err = TxUpdateTaskStatus(tx, task, tstate, time.Now(), msg)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "TX Update Node Status "+n.ID)
	}

	n.Status = nstate

	return nil
}

func GetNode(nameOrID string) (Node, error) {
	db, err := GetDB(false)
	if err != nil {
		return Node{}, err
	}

	node := Node{}
	query := "SELECT * FROM tb_node WHERE id=? OR name=? OR engine_id=?"

	err = db.Get(&node, query, nameOrID, nameOrID, nameOrID)
	if err == nil {
		return node, nil
	}
	if _err := CheckError(err); _err == ErrNoRowsFound {
		return node, errors.Wrap(err, "Not Found Node By:"+nameOrID)
	}

	db, err = GetDB(true)
	if err != nil {
		return Node{}, err
	}

	err = db.Get(&node, query, nameOrID, nameOrID, nameOrID)
	if err == nil {
		return node, nil
	}

	return node, errors.Wrap(err, "Get Node By:"+nameOrID)
}

func GetAllNodes() ([]Node, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	query := "SELECT * FROM tb_node"

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

	return nil, errors.Wrap(err, "Get All Nodes")
}

func ListNodeByCluster(cluster string) ([]*Node, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	query := "SELECT * FROM tb_node WHERE cluster_id=?"

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

	return nil, errors.Wrap(err, "List Node By Cluster:"+cluster)
}

func CountNodeByCluster(cluster string) (int, error) {
	db, err := GetDB(false)
	if err != nil {
		return 0, err
	}

	num := 0
	query := "SELECT COUNT(*) FROM tb_node WHERE cluster_id=?"

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

	return 0, errors.Wrap(err, "Count Node By Cluster:"+cluster)
}

func ListNodesByEngines(names []string) ([]Node, error) {
	if len(names) == 0 {
		return nil, nil
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

	return nil, errors.Wrapf(err, "List Nodes By Engines:%s", names)
}

func ListNodesByClusters(clusters []string, _type string, enable bool) ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var clist []string
	err = db.Select(&clist, "SELECT id FROM tb_cluster WHERE type=? AND enabled=?", _type, enable)
	if err != nil {
		return nil, errors.Wrapf(err, "List Cluster by Type='%s',enabled=%t", _type, enable)
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
		return nil, errors.New("Cluster List is nil")
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

	return nil, errors.Wrap(err, "List Nodes By Clusters")
}

func DeleteNode(nameOrID string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	query := "DELETE FROM tb_node WHERE id=? OR name=?"
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

	return errors.Wrap(err, "Delete Node By "+nameOrID)
}
