package database

import (
	"database/sql"
	"net"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type NodeOrmer interface {
	NodeIface
	GetSysConfigIface
	VolumeOrmer
	StorageIface
}

type NodeIface interface {
	InsertNodesAndTask(nodes []Node, tasks []Task) error

	GetNode(nameOrID string) (Node, error)

	ListNodes() ([]Node, error)

	ListNodesByTag(tag string) ([]Node, error)

	ListNodesByTags(tags []string, enable bool) ([]Node, error)

	//	CountNodeByTag(tag string) (int, error)
	CountUnitByEngine(id string) (int, error)

	SetNodeEnable(string, bool) error
	SetNodeParam(ID string, max int, usage float32) error

	RegisterNode(n *Node, t *Task) error

	DelNode(nameOrID string) error
}

// Node table structure,correspod with mainframe computer.
type Node struct {
	Enabled      bool    `db:"enabled"`
	UsageMax     float32 `db:"usage_max"`
	ContainerMax int     `db:"max_container"`
	Status       int     `db:"status"`

	ID               string `db:"id"`
	Tag              string `db:"tag"`
	Addr             string `db:"admin_ip"`
	EngineID         string `db:"engine_id"`
	Room             string `db:"room"`
	Seat             string `db:"seat"`
	Storage          string `db:"storage"`
	NetworkPartition string `db:"ha_network_tag"`

	NFS

	RegisterAt time.Time `db:"register_at"`
}

// NFSOption nfs settings
type NFS struct {
	Addr     string `db:"nfs_ip"`
	Dir      string `db:"nfs_dir"`
	MountDir string `db:"nfs_mount_dir"`
	Options  string `db:"nfs_mount_opts"`
}

func (db dbBase) nodeTable() string {
	return db.prefix + "_host"
}

// InsertNodesAndTask insert nodes and tasks in a Tx
func (db dbBase) InsertNodesAndTask(nodes []Node, tasks []Task) error {
	do := func(tx *sqlx.Tx) error {

		query := "INSERT INTO " + db.nodeTable() + " (id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts) VALUES (:id,:tag,:admin_ip,:engine_id,:room,:seat,:storage,:ha_network_tag,:usage_max,:max_container,:status,:enabled,:register_at,:nfs_ip,:nfs_dir,:nfs_mount_dir,:nfs_mount_opts)"

		if len(nodes) == 1 {
			_, err := tx.NamedExec(query, &nodes[0])
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
				_, err = stmt.Exec(&nodes[i])
				if err != nil {
					stmt.Close()

					return errors.Wrap(err, "tx prepare insert []Node")
				}
			}

			stmt.Close()
		}

		return db.InsertTasks(tx, tasks, db.nodeTable())
	}

	return db.txFrame(do)
}

// SetNodeParams returns error when Node update usage_max and max_container.
func (db dbBase) SetNodeParam(ID string, max int, usage float32) error {

	query := "UPDATE " + db.nodeTable() + " SET max_container=?,usage_max=? WHERE id=?"

	_, err := db.Exec(query, max, usage, ID)

	return errors.Wrap(err, "update Node MaxContainer & UsageMax by ID")
}

// SetNodeParams returns error when Node update enabled.
func (db dbBase) SetNodeEnable(ID string, enabled bool) error {

	query := "UPDATE " + db.nodeTable() + " SET enabled=? WHERE id=?"

	_, err := db.Exec(query, enabled, ID)

	return errors.Wrap(err, "update Node.Enabled by ID")
}

// RegisterNode returns error when Node UPDATE infomation.
func (db dbBase) RegisterNode(n *Node, t *Task) error {
	do := func(tx *sqlx.Tx) (err error) {
		if n != nil {
			query := "UPDATE " + db.nodeTable() + " SET engine_id=?,status=?,enabled=?,register_at=? WHERE id=?"

			_, err = tx.Exec(query, n.EngineID, n.Status, n.Enabled, n.RegisterAt, n.ID)
			if err != nil {
				return errors.Wrap(err, "Tx update Node status")
			}
		}

		if t != nil {
			return db.txSetTask(tx, *t)
		}

		return nil
	}

	return db.txFrame(do)
}

// GetNode get Node by nameOrID.
func (db dbBase) GetNode(nameOrID string) (Node, error) {
	var node Node
	query := "SELECT id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at, nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts FROM " + db.nodeTable() + " WHERE id=? OR engine_id=?"

	err := db.Get(&node, query, nameOrID, nameOrID)

	return node, errors.Wrap(err, "get Node by:"+nameOrID)
}

// GetNodeByAddr returns Node by addr.
func (db dbBase) GetNodeByAddr(addr string) (Node, error) {
	var (
		node  Node
		query = "SELECT id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts FROM " + db.nodeTable() + " WHERE admin_ip=?"
	)

	addr, _, err := net.SplitHostPort(addr)
	if err != nil {
		return node, errors.WithStack(err)
	}

	err = db.Get(&node, query, addr)

	return node, errors.Wrap(err, "get Node by addr")
}

// ListNodes returns all nodes.
func (db dbBase) ListNodes() ([]Node, error) {
	var (
		nodes []Node
		query = "SELECT id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts FROM " + db.nodeTable()
	)

	err := db.Select(&nodes, query)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return nodes, errors.Wrap(err, "get all Nodes")
}

// ListNodeByTag returns nodes,select by tag
func (db dbBase) ListNodesByTag(tag string) ([]Node, error) {
	var (
		nodes []Node
		query = "SELECT id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts FROM " + db.nodeTable() + " WHERE tag=?"
	)

	err := db.Select(&nodes, query, tag)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return nodes, errors.Wrap(err, "list Node by tag")
}

// CountNodeByTag returns num of node select by tag.
func (db dbBase) CountNodeByTag(tag string) (int, error) {
	num := 0
	query := "SELECT COUNT(id) FROM " + db.nodeTable() + " WHERE tag=?"

	err := db.Get(&num, query, tag)

	return num, errors.Wrap(err, "count Node by Tag")
}

// ListNodesByEngines returns nodes,select by engines ID.
func (db dbBase) ListNodesByEngines(names []string) ([]Node, error) {
	if len(names) == 0 {
		return []Node{}, nil
	}

	var (
		nodes []Node
		query = "SELECT id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts FROM " + db.nodeTable() + " WHERE engine_id IN (?);"
	)

	query, args, err := sqlx.In(query, names)
	if err != nil {
		return nil, errors.Wrap(err, "select []Node IN engines")
	}

	err = db.Select(&nodes, query, args...)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return nodes, errors.Wrapf(err, "list Nodes by engines:%s", names)
}

// ListNodesByIDs returns nodes,select by ID.
func (db dbBase) ListNodesByIDs(in []string, tag string) ([]Node, error) {
	if len(in) == 0 {
		return db.ListNodesByTag(tag)
	}

	var (
		nodes []Node
		query = "SELECT id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts FROM " + db.nodeTable() + " WHERE id IN (?);"
	)

	query, args, err := sqlx.In(query, in)
	if err != nil {
		return nil, errors.Wrap(err, "select []Node IN IDs")
	}

	err = db.Select(&nodes, query, args...)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return nodes, errors.Wrapf(err, "list Nodes by IDs:%s", in)
}

// ListNodesByTags returns nodes,select by tags\type\enabled.
func (db dbBase) ListNodesByTags(tags []string, enable bool) ([]Node, error) {
	if len(tags) == 0 {
		return []Node{}, errors.New("tag is required")
	}

	query := "SELECT id,tag,admin_ip,engine_id,room,seat,storage,ha_network_tag,usage_max,max_container,status,enabled,register_at,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts FROM " + db.nodeTable() + " WHERE tag IN (?) AND enabled=?;"
	query, args, err := sqlx.In(query, tags, enable)
	if err != nil {
		return nil, errors.Wrap(err, "select []Node IN tags")
	}

	var nodes []Node
	err = db.Select(&nodes, query, args...)

	return nodes, errors.Wrap(err, "list Nodes by tags")
}

// DelNode delete node by ID
func (db dbBase) DelNode(ID string) error {

	query := "DELETE FROM " + db.nodeTable() + " WHERE id=?"

	_, err := db.Exec(query, ID)

	return errors.Wrap(err, "delete Node by ID")
}
