package database

import (
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type NodeOrmer interface {
	ClusterInterface
	NodeInterface
	SysConfigOrmer
	VolumeOrmer
}

type NodeInterface interface {
	InsertNodesAndTask(nodes []Node, tasks []Task) error

	GetNode(nameOrID string) (Node, error)

	ListNodes() ([]Node, error)

	ListNodeByCluster(cluster string) ([]Node, error)
	ListNodesByClusters(clusters []string, _type string, enable bool) ([]Node, error)

	SetNodeParams(n Node) error

	RegisterNode(n Node, t Task) error

	DelNode(nameOrID string) error
}

// Node table structure,correspod with mainframe computer.
type Node struct {
	ID string `db:"id"`
	//	Name         string `db:"name"`
	ClusterID    string `db:"cluster_id"`
	Addr         string `db:"admin_ip"`
	EngineID     string `db:"engine_id"`
	Room         string `db:"room"`
	Seat         string `db:"seat"`
	MaxContainer int    `db:"max_container"`
	Status       int    `db:"status"`
	Enabled      bool   `db:"enabled"`

	RegisterAt   time.Time `db:"register_at"`
	DeregisterAt time.Time `db:"deregister_at"`
}

func (db dbBase) nodeTable() string {
	return db.prefix + "_host"
}

// InsertNodesAndTask insert nodes and tasks in a Tx
func (db dbBase) InsertNodesAndTask(nodes []Node, tasks []Task) error {
	do := func(tx *sqlx.Tx) error {

		query := "INSERT INTO " + db.nodeTable() + " (id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at) VALUES (:id,:name,:cluster_id,:admin_ip,:engine_id,:room,:seat,:max_container,:status,:register_at,:deregister_at)"

		if len(nodes) == 1 {
			_, err := tx.Exec(query, nodes[0])
			if err != nil {
				return errors.Wrap(err, "Tx insert Node")
			}

		} else {
			// use prepare insert into database
			stmt, err := tx.PrepareNamed(query)
			if err != nil {
				return errors.Wrap(err, "tx prepare insert Node")
			}

			for i := range nodes {
				_, err = stmt.Exec(nodes[i])
				if err != nil {
					stmt.Close()

					return errors.Wrap(err, "tx prepare insert []Node")
				}
			}
			stmt.Close()
		}

		err := db.InsertTasks(tx, tasks)

		return err
	}

	return db.txFrame(do)
}

// SetNodeParams returns error when Node update status and max_container.
func (db dbBase) SetNodeParams(n Node) error {

	query := "UPDATE " + db.nodeTable() + " SET status=?,max_container=? WHERE id=?"

	_, err := db.Exec(query, n.Status, n.MaxContainer, n.ID)

	return errors.Wrap(err, "update Node Params by ID")
}

// RegisterNode returns error when Node UPDATE infomation.
func (db dbBase) RegisterNode(n Node, t Task) error {
	do := func(tx *sqlx.Tx) (err error) {

		query := "UPDATE " + db.nodeTable() + " SET engine_id=?,status=?,register_at=? WHERE id=?"

		_, err = tx.Exec(query, n.EngineID, n.Status, n.RegisterAt, n.ID)
		if err != nil {
			return errors.Wrap(err, "Tx update Node status")
		}

		err = db.txSetTask(tx, t)

		return err
	}

	err := db.txFrame(do)

	return err
}

// GetNode get Node by nameOrID.
func (db dbBase) GetNode(nameOrID string) (Node, error) {
	var (
		node  Node
		query = "SELECT id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at FROM " + db.nodeTable() + " WHERE id=? OR name=? OR engine_id=?"
	)

	err := db.Get(&node, query, nameOrID, nameOrID, nameOrID)
	if err == nil {
		return node, nil
	}
	if err == sql.ErrNoRows {
		return node, errors.Wrap(err, "not found Node by:"+nameOrID)
	}

	return node, errors.Wrap(err, "get Node by:"+nameOrID)
}

// GetNodeByAddr returns Node by addr.
func (db dbBase) GetNodeByAddr(addr string) (Node, error) {
	var (
		node  Node
		query = "SELECT id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at FROM " + db.nodeTable() + " WHERE admin_ip=?"
	)

	err := db.Get(&node, query, addr)
	if err == nil {
		return node, nil
	}
	if err == sql.ErrNoRows {
		return node, errors.Wrap(err, "not found Node by addr:"+addr)
	}

	return node, errors.Wrap(err, "get Node by addr")
}

// ListNodes returns all nodes.
func (db dbBase) ListNodes() ([]Node, error) {
	var (
		nodes []Node
		query = "SELECT id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at FROM " + db.nodeTable()
	)

	err := db.Select(&nodes, query)

	return nodes, errors.Wrap(err, "get all Nodes")
}

// ListNodeByCluster returns nodes,select by cluster
func (db dbBase) ListNodeByCluster(cluster string) ([]Node, error) {
	var (
		nodes []Node
		query = "SELECT id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at FROM " + db.nodeTable() + " WHERE cluster_id=?"
	)

	err := db.Select(&nodes, query, cluster)

	return nodes, errors.Wrap(err, "list Node by cluster")
}

// CountNodeByCluster returns num of node select by cluster.
func (db dbBase) CountNodeByCluster(cluster string) (int, error) {
	var (
		num   = 0
		query = "SELECT COUNT(id) FROM " + db.nodeTable() + " WHERE cluster_id=?"
	)

	err := db.Get(&num, query, cluster)

	return num, errors.Wrap(err, "count Node by cluster")
}

// ListNodesByEngines returns nodes,select by engines ID.
func (db dbBase) ListNodesByEngines(names []string) ([]Node, error) {
	if len(names) == 0 {
		return []Node{}, nil
	}

	var (
		nodes []Node
		query = "SELECT id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at FROM " + db.nodeTable() + " WHERE engine_id IN (?);"
	)

	query, args, err := sqlx.In(query, names)
	if err != nil {
		return nil, errors.Wrap(err, "select []Node IN engines")
	}

	err = db.Select(&nodes, query, args...)

	return nodes, errors.Wrapf(err, "list Nodes by engines:%s", names)
}

// ListNodesByIDs returns nodes,select by ID.
func (db dbBase) ListNodesByIDs(in []string, cluster string) ([]Node, error) {
	if len(in) == 0 {
		return db.ListNodeByCluster(cluster)
	}

	var (
		nodes []Node
		query = "SELECT id,name,cluster_id,admin_ip,engine_id,room,seat,max_container,status,register_at,deregister_at FROM " + db.nodeTable() + " WHERE id IN (?);"
	)

	query, args, err := sqlx.In(query, in)
	if err != nil {
		return nil, errors.Wrap(err, "select []Node IN IDs")
	}

	err = db.Select(&nodes, query, args...)

	return nodes, errors.Wrapf(err, "list Nodes by IDs:%s", in)
}

// ListNodesByClusters returns nodes,select by clusters\type\enabled.
func (db dbBase) ListNodesByClusters(clusters []string, _type string, enable bool) ([]Node, error) {
	list := make([]string, 0, len(clusters))

	for _, c := range clusters {
		if len(c) == 0 || strings.TrimSpace(c) == "" {
			continue
		}
		list = append(list, c)
	}

	clusters = list

	if len(clusters) == 0 {
		query := "SELECT id FROM " + db.clusterTable() + " WHERE type=? AND enabled=?"
		err := db.Select(&clusters, query, _type, enable)
		if err != nil {
			return nil, errors.Wrapf(err, "list Cluster by type='%s',enabled=%t", _type, enable)
		}
	}

	if len(clusters) == 0 {
		return []Node{}, nil
	}

	query := "SELECT id,name,type,storage_id,storage_type,networking_id,enabled,max_node,usage_limit FROM " + db.clusterTable() + " WHERE cluster_id IN (?);"
	query, args, err := sqlx.In(query, clusters)
	if err != nil {
		return nil, errors.Wrap(err, "select []Node IN clusterIDs")
	}

	var nodes []Node
	err = db.Select(&nodes, query, args...)

	return nodes, errors.Wrap(err, "list Nodes by clusters")
}

// DelNode delete node by name or ID
func (db dbBase) DelNode(nameOrID string) error {

	query := "DELETE FROM " + db.nodeTable() + " WHERE id=? OR name=?"

	_, err := db.Exec(query, nameOrID, nameOrID)

	return errors.Wrap(err, "delete Node by nameOrID")
}
