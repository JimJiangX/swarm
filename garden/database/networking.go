package database

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// TODO: combine Networking and IP

type NetworkingOrmer interface {
	// IP
	ListIPByNetworking(networkingID string) ([]IP, error)
	ListIPByUnitID(unit string) ([]IP, error)
	ListIPWithCondition(networking string, allocation bool, num int) ([]IP, error)

	AllocIPByNetworking(networkingID, _type, unit string) (IP, error)

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
	Allocated    bool   `db:"allocated"`
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       int    `db:"prefix"`
	ID           string `db:"id"`
	NetworkingID string `db:"networking_id"`
	Type         string `db:"type"`
	UnitID       string `db:"unit_id"`
	Gateway      string `db:"gateway"`
}

func (db dbBase) ipTable() string {
	return db.prefix + "_ip"
}

// ListIPByNetworking returns []IP select by NetworkingID
func (db dbBase) ListIPByNetworking(networkingID string) ([]IP, error) {
	var (
		list  []IP
		query = "SELECT id,allocated,ip_addr,prefix,networking_id,type,unit_id,gateway FROM " + db.ipTable() + " WHERE networking_id=?"
	)

	err := db.Select(&list, query, networkingID)

	return list, errors.Wrap(err, "list []IP by networking")
}

// ListIPWithCondition returns []IP select by  Allocated==allocated
func (db dbBase) listIPsByAllocated(allocation bool, num int) ([]IP, error) {
	var (
		out   []IP
		query string
	)

	if num > 0 {
		query = fmt.Sprintf("SELECT id,allocated,ip_addr,prefix,networking_id,type,unit_id,gateway from %s WHERE allocated=? LIMIT %d", db.ipTable(), num)
	} else {
		query = fmt.Sprintf("SELECT id,allocated,ip_addr,prefix,networking_id,type,unit_id,gateway from %s WHERE allocated=?", db.ipTable())
	}

	err := db.Select(&out, query, allocation)

	return out, errors.Wrap(err, "list []IP by allocated")
}

// ListIPByUnitID returns []IP select by UnitID
func (db dbBase) ListIPByUnitID(unit string) ([]IP, error) {
	var (
		out   []IP
		query = "SELECT id,allocated,ip_addr,prefix,networking_id,type,unit_id,gateway from " + db.ipTable() + " WHERE unit_id=?"
	)

	err := db.Select(&out, query, unit)

	return out, errors.Wrap(err, "list []IP by UnitID")
}

// ListIPWithCondition returns []IP select by NetworkingID and Allocated==allocated
func (db dbBase) ListIPWithCondition(networking string, allocation bool, num int) ([]IP, error) {
	var (
		out   []IP
		query = fmt.Sprintf("SELECT id,allocated,ip_addr,prefix,networking_id,type,unit_id,gateway from %s WHERE networking_id=? AND allocated=? LIMIT %d", db.ipTable(), num)
	)

	err := db.Select(&out, query, networking, allocation)

	return out, errors.Wrap(err, "list []IP with condition")
}

// TxAllocIPByNetworking update IP UnitID in Tx
func (db dbBase) AllocIPByNetworking(networking, _type, unit string) (IP, error) {
	out := &IP{}

	do := func(tx *sqlx.Tx) error {

		if networking != "" {

			query := "SELECT id,ip_addr,prefix,gateway FROM " + db.ipTable() + " WHERE networking_id=? AND allocated=? LIMIT 1 FOR UPDATE;"
			query = tx.Rebind(query)

			err := tx.Get(out, query, networking, false)
			if err != nil {
				return errors.Wrap(err, "Tx get available IP")
			}

		} else if _type != "" {

			query := "SELECT id,ip_addr,prefix,gateway FROM " + db.ipTable() + " WHERE type=? AND allocated=? LIMIT 1 FOR UPDATE;"
			query = tx.Rebind(query)

			err := tx.Get(out, query, _type, false)
			if err != nil {
				return errors.Wrap(err, "Tx get available IP")
			}

		} else {
			return errors.New("neither networkingID nor type is null")
		}

		out.Allocated = true
		out.UnitID = unit

		query := "UPDATE " + db.ipTable() + " SET allocated=?,unit_id=? WHERE id=?"

		_, err := tx.Exec(query, true, unit, out.ID)

		return errors.Wrap(err, "tx update IP")
	}

	err := db.txFrame(do)

	return *out, errors.Wrap(err, "tx update IP")
}

// txSetIPs update []IP in Tx
func (db dbBase) txSetIPs(tx *sqlx.Tx, val []IP) error {
	query := "UPDATE " + db.ipTable() + " SET unit_id=:unit_id,allocated=:allocated WHERE ip_addr=:ip_addr AND prefix=:prefix"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare update []IP")
	}

	for i := range val {
		_, err = stmt.Exec(&val[i])
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

		query := "INSERT INTO " + db.ipTable() + " (id,allocated,ip_addr,prefix,networking_id,type,unit_id,gateway) VALUES (:id,:allocated,:ip_addr,:prefix,:networking_id,:type,:unit_id,:gateway)"

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
// one more IP belongs to networking has allocated
func (db dbBase) IsNetwrokingUsed(networking string) (bool, error) {
	var (
		count = 0
		query = "SELECT COUNT(id) from " + db.ipTable() + " WHERE networking_id=? AND allocated=?"
	)

	err := db.Get(&count, query, networking, true)
	if err != nil {
		return false, errors.Wrap(err, "count []IP by NetworkingID")
	}

	return count > 0, nil
}
