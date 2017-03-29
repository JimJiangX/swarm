package database

import (
	"database/sql"

	"github.com/pkg/errors"
)

type ClusterInterface interface {
	InsertCluster(c Cluster) error

	GetCluster(ID string) (Cluster, error)

	ListClusters() ([]Cluster, error)

	SetClusterParams(c Cluster) error

	DelCluster(ID string) error
}

type ClusterOrmer interface {
	ClusterInterface
	NodeInterface
	SysConfigOrmer
}

// Cluster table  structure,correspod with a group of computers
type Cluster struct {
	ID         string  `db:"id"`
	MaxNode    int     `db:"max_host"`
	UsageLimit float32 `db:"usage_limit"`
}

func (db dbBase) clusterTable() string {
	return db.prefix + "_cluster"
}

// InsertCluster insert a new record.
func (db dbBase) InsertCluster(c Cluster) error {
	query := "INSERT INTO " + db.clusterTable() + " (id,max_host,usage_limit) VALUES (:id,:max_host,:usage_limit)"

	_, err := db.NamedExec(query, &c)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "insert Cluster")
}

// GetCluster get Cluster by nameOrID.
func (db dbBase) GetCluster(ID string) (Cluster, error) {
	var (
		c     Cluster
		query = "SELECT id,max_host,usage_limit FROM " + db.clusterTable() + " WHERE id=?"
	)

	err := db.Get(&c, query, ID)
	if err == nil {
		return c, nil

	} else if err == sql.ErrNoRows {
		return c, errors.Wrap(err, "not found Cluster:"+ID)
	}

	return c, errors.Wrap(err, "get Cluster")
}

// ListClusters select Cluster
func (db dbBase) ListClusters() ([]Cluster, error) {
	var (
		clusters []Cluster
		query    = "SELECT id,max_host,usage_limit FROM " + db.clusterTable()
	)

	err := db.Select(&clusters, query)
	if err == nil {
		return clusters, nil
	} else if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, errors.Wrap(err, "list Clusters")
}

// SetClusterParams updates MaxNode\UsageLimit
func (db dbBase) SetClusterParams(c Cluster) error {

	query := "UPDATE " + db.clusterTable() + " SET max_host=?,usage_limit=? WHERE id=?"

	_, err := db.Exec(query, c.MaxNode, c.UsageLimit, c.ID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "update Cluster MaxNode or UsageLimit")
}

// DelCluster delete a record of Cluster by ID
func (db dbBase) DelCluster(ID string) error {

	query := "DELETE FROM " + db.clusterTable() + " WHERE id=?"

	_, err := db.Exec(query, ID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "delete Cluster")
}
