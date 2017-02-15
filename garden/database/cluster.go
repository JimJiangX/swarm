package database

import (
	"database/sql"

	"github.com/pkg/errors"
)

type ClusterInterface interface {
	InsertCluster(c Cluster) error

	GetCluster(nameOrID string) (Cluster, error)

	ListClusters() ([]Cluster, error)

	SetClusterStatus(nameOrID string, state bool) error

	DelCluster(nameOrID string) error
}

type ClusterOrmer interface {
	ClusterInterface
	NodeInterface
	SysConfigOrmer
}

// Cluster table  structure,correspod with a group of computers
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

func (db dbBase) clusterTable() string {
	return db.prefix + "_cluster"
}

// InsertCluster insert a new record.
func (db dbBase) InsertCluster(c Cluster) error {
	query := "INSERT INTO " + db.clusterTable() + " (id,name,type,storage_id,storage_type,networking_id,enabled,max_node,usage_limit) VALUES (:id,:name,:type,:storage_id,:storage_type,:networking_id,:enabled,:max_node,:usage_limit)"

	_, err := db.NamedExec(query, &c)

	return errors.Wrap(err, "insert Cluster")
}

// GetCluster get Cluster by nameOrID.
func (db dbBase) GetCluster(nameOrID string) (Cluster, error) {
	var (
		c     Cluster
		query = "SELECT id,name,type,storage_id,storage_type,networking_id,enabled,max_node,usage_limit FROM " + db.clusterTable() + " WHERE id=? OR name=?"
	)

	err := db.Get(&c, query, nameOrID, nameOrID)
	if err == nil {
		return c, nil

	} else if err == sql.ErrNoRows {
		return c, errors.Wrap(err, "not found Cluster:"+nameOrID)
	}

	return c, errors.Wrap(err, "get Cluster")
}

// ListClusters select Cluster
func (db dbBase) ListClusters() ([]Cluster, error) {
	var (
		clusters []Cluster
		query    = "SELECT id,name,type,storage_id,storage_type,networking_id,enabled,max_node,usage_limit FROM " + db.clusterTable()
	)

	err := db.Select(&clusters, query)

	return clusters, errors.Wrap(err, "list Clusters")
}

// SetClusterStatus update Cluster.enabled by ID
func (db dbBase) SetClusterStatus(nameOrID string, state bool) error {

	query := "UPDATE " + db.clusterTable() + " SET enabled=? WHERE id=? OR name=?"

	_, err := db.Exec(query, state, nameOrID, nameOrID)

	return errors.Wrap(err, "update Cluster.Enabled by nameOrID:"+nameOrID)
}

// SetClusterParams updates MaxNode\UsageLimit
func (db dbBase) SetClusterParams(c Cluster) error {

	query := "UPDATE " + db.clusterTable() + " SET max_node=?,usage_limit=? WHERE id=?"

	_, err := db.Exec(query, c.MaxNode, c.UsageLimit, c.ID)

	return errors.Wrap(err, "update Cluster MaxNode or UsageLimit")
}

// DelCluster delete a record of Cluster by nameOrID
func (db dbBase) DelCluster(nameOrID string) error {

	query := "DELETE FROM " + db.clusterTable() + " WHERE id=? OR name=?"

	_, err := db.Exec(query, nameOrID, nameOrID)

	return errors.Wrap(err, "delete Cluster")
}
