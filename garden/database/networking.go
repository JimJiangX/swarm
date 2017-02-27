package database

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type NetworkingRequire struct {
	Networking string
	Num        int
}

type NetworkingOrmer interface {
	// IP
	ListIPByNetworking(networkingID string) ([]IP, error)
	ListIPByUnitID(unit string) ([]IP, error)
	// ListIPWithCondition(networking string, allocation bool, num int) ([]IP, error)

	AllocNetworking(req []NetworkingRequire, unit string) ([]IP, error)

	// Networking
	InsertNetworking([]IP) error
	DelNetworking(ID string) error
	IsNetwrokingUsed(networking string) (bool, error)
	// Port
	//	ImportPort(start, end int, filter ...int) (int, error)

	//	ListAvailablePorts(num int) ([]Port, error)
	//	ListPortsByUnit(nameOrID string) ([]Port, error)
	//	ListPorts(start, end, limit int) ([]Port, error)

	//	DelPort(port int, allocated bool) error

	// Networking

	//	SetNetworkingStatus(ID string, enable bool) error

	//	GetNetworkingByID(ID string) (Networking, int, error)
	//	ListNetworking() ([]Networking, error)
	//	ListNetworkingByType(_type string) ([]Networking, error)
}

// IP is table structure, associate to Networking
type IP struct {
	Enabled    bool   `db:"enabled"`
	IPAddr     uint32 `db:"ip_addr"`
	Prefix     int    `db:"prefix"`
	Networking string `db:"networking_id"`
	UnitID     string `db:"unit_id"`
	Gateway    string `db:"gateway"`
	VLAN       string `db:"vlan_id"`
}

// ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled
func (db dbBase) ipTable() string {
	return db.prefix + "_ip"
}

// ListIPByNetworking returns []IP select by networking
func (db dbBase) ListIPByNetworking(networking string) ([]IP, error) {
	var (
		list  []IP
		query = "SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled FROM " + db.ipTable() + " WHERE networking_id=?"
	)

	err := db.Select(&list, query, networking)

	return list, errors.Wrap(err, "list []IP by networking")
}

// ListIPWithCondition returns []IP select by  unit_id!=""
func (db dbBase) listIPsByAllocated(allocated bool, num int) ([]IP, error) {
	var (
		out   []IP
		query string
	)

	condition := "IS NOT NULL"
	if !allocated {
		condition = "IS NULL"
	}

	if num > 0 {
		query = fmt.Sprintf("SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled FROM %s WHERE unit_id %s LIMIT %d", db.ipTable(), condition, num)
	} else {
		query = fmt.Sprintf("SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled FROM %s WHERE unit_id %s", db.ipTable(), condition)
	}

	err := db.Select(&out, query)

	return out, errors.Wrap(err, "list []IP by allocated")
}

// ListIPByUnitID returns []IP select by UnitID
func (db dbBase) ListIPByUnitID(unit string) ([]IP, error) {
	var (
		out   []IP
		query = "SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled FROM " + db.ipTable() + " WHERE unit_id=?"
	)

	err := db.Select(&out, query, unit)

	return out, errors.Wrap(err, "list []IP by UnitID")
}

// ListIPWithCondition returns []IP select by NetworkingID and Allocated==allocated
func (db dbBase) ListIPWithCondition(networking string, allocated bool, num int) ([]IP, error) {
	var (
		out       []IP
		condition = "IS NOT NULL"
	)

	if !allocated {
		condition = "IS NULL"
	}

	query := fmt.Sprintf("SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled FROM %s WHERE networking_id=? AND unit_id %s LIMIT %d", db.ipTable(), condition, num)

	err := db.Select(&out, query, networking)

	return out, errors.Wrap(err, "list []IP with condition")
}

// AllocNetworking alloc IPs with UnitID in Tx
func (db dbBase) AllocNetworking(requires []NetworkingRequire, unit string) ([]IP, error) {
	out := make([]IP, 0, 2*len(requires))

	do := func(tx *sqlx.Tx) error {

		for _, req := range requires {

			query := fmt.Sprintf("SELECT ip_addr,prefix,gateway,vlan_id,networking_id FROM %s WHERE networking_id=? AND enabled=? AND unit_id IS NULL LIMIT %d FOR UPDATE;", db.ipTable(), req.Num)
			query = tx.Rebind(query)

			var list []IP
			err := tx.Select(&list, query, req.Networking, true)
			if err != nil {
				return errors.Wrap(err, "Tx get available IP")
			}

			out = append(out, list...)
		}

		for i := range out {
			out[i].UnitID = unit
		}

		return db.txSetIPs(tx, out)
	}

	err := db.txFrame(do)

	return out, err
}

// txSetIPs update []IP in Tx
func (db dbBase) txSetIPs(tx *sqlx.Tx, val []IP) error {
	query := "UPDATE " + db.ipTable() + " SET unit_id=? WHERE ip_addr=?"

	stmt, err := tx.Prepare(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare update []IP")
	}

	for i := range val {
		_, err = stmt.Exec(val[i].UnitID, val[i].IPAddr)
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx update IP")
		}
	}

	err = stmt.Close()

	return errors.Wrap(err, "Tx update []IP")
}

func (db dbBase) InsertNetworking(ips []IP) error {
	do := func(tx *sqlx.Tx) error {

		query := "INSERT INTO " + db.ipTable() + " ( ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled ) VALUES ( :ip_addr,:prefix,:networking_id,:unit_id,:gateway,:vlan_id,:enabled )"

		stmt, err := tx.PrepareNamed(query)
		if err != nil {
			return errors.Wrap(err, "Tx prepare insert []IP")
		}

		for i := range ips {

			_, err = stmt.Exec(&ips[i])
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "Tx insert IP")
			}
		}

		stmt.Close()

		return errors.Wrap(err, "Tx insert Networking")
	}

	return db.txFrame(do)
}

// DelNetworking delete Networking and []IP in Tx
func (db dbBase) DelNetworking(networking string) error {
	do := func(tx *sqlx.Tx) error {

		_, err := tx.Exec("DELETE FROM "+db.ipTable()+" WHERE networking_id=?", networking)

		return errors.Wrap(err, "Tx delete []IP by NetworkingID")
	}

	return db.txFrame(do)
}

// IsNetwrokingUsed returns true on below conditions:
// one or more IP belongs to networking has allocated
func (db dbBase) IsNetwrokingUsed(networking string) (bool, error) {
	var (
		count = 0
		query = "SELECT COUNT(id) FROM " + db.ipTable() + " WHERE networking_id=? AND unit_id IS NOT NULL"
	)

	err := db.Get(&count, query, networking)
	if err != nil {
		return false, errors.Wrap(err, "count []IP by NetworkingID")
	}

	return count > 0, nil
}
