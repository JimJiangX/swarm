package database

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type NetworkingRequire struct {
	Networking string
	Bond       string
	Bandwidth  int // M/s
}

type NetworkingOrmer interface {
	// IP
	ListIPByNetworking(networkingID string) ([]IP, error)
	ListIPByUnitID(unit string) ([]IP, error)
	ListIPByEngine(ID string) ([]IP, error)

	AllocNetworking(unit, engine string, req []NetworkingRequire) ([]IP, error)

	InsertNetworking([]IP) error
	DelNetworking(ID string) error

	SetNetworkingEnable(ID string, enable bool) error
	SetIPEnable([]uint32, string, bool) error
}

// IP is table structure, associate to Networking
type IP struct {
	Enabled    bool   `db:"enabled"`
	IPAddr     uint32 `db:"ip_addr"`
	Prefix     int    `db:"prefix"`
	VLAN       int    `db:"vlan_id"`
	Networking string `db:"networking_id"`
	UnitID     string `db:"unit_id"`
	Gateway    string `db:"gateway"`
	Engine     string `db:"engine_id"`
	Bond       string `db:"net_dev"`
	Bandwidth  int    `db:"bandwidth"`
}

// ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled
func (db dbBase) ipTable() string {
	return db.prefix + "_ip"
}

// ListIPByNetworking returns []IP select by networking
func (db dbBase) ListIPByNetworking(networking string) ([]IP, error) {
	var (
		list  []IP
		query = "SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled,engine_id,net_dev,bandwidth FROM " + db.ipTable() + " WHERE networking_id=?"
	)

	err := db.Select(&list, query, networking)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return list, errors.Wrap(err, "list []IP by networking")
}

// ListIPWithCondition returns []IP select by  unit_id!=""
func (db dbBase) listIPsByAllocated(allocated bool, num int) ([]IP, error) {
	var (
		out   []IP
		query string
	)

	opt := "<>"
	if !allocated {
		opt = "="
	}

	if num > 0 {
		query = fmt.Sprintf("SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled,engine_id,net_dev,bandwidth FROM %s WHERE unit_id%s? LIMIT %d", db.ipTable(), opt, num)
	} else {
		query = fmt.Sprintf("SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled,engine_id,net_dev,bandwidth FROM %s WHERE unit_id%s?", db.ipTable(), opt)
	}

	err := db.Select(&out, query, "")
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []IP by allocated")
}

// ListIPByUnitID returns []IP select by UnitID
func (db dbBase) ListIPByUnitID(unit string) ([]IP, error) {
	var (
		out   []IP
		query = "SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled,engine_id,net_dev,bandwidth FROM " + db.ipTable() + " WHERE unit_id=?"
	)

	err := db.Select(&out, query, unit)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []IP by UnitID")
}

// ListIPByUnitID returns []IP select by Engine
func (db dbBase) ListIPByEngine(ID string) ([]IP, error) {
	var (
		out   []IP
		query = "SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled,engine_id,net_dev,bandwidth FROM " + db.ipTable() + " WHERE engine_id=?"
	)

	err := db.Select(&out, query, ID)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []IP by EngineID")
}

// ListIPWithCondition returns []IP select by NetworkingID and Allocated==allocated
func (db dbBase) ListIPWithCondition(networking string, allocated bool, num int) ([]IP, error) {
	var (
		out []IP
		opt = "<>"
	)

	if !allocated {
		opt = "="
	}

	query := fmt.Sprintf("SELECT ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled,engine_id,net_dev,bandwidth FROM %s WHERE networking_id=? AND unit_id%s? LIMIT %d", db.ipTable(), opt, num)

	err := db.Select(&out, query, networking, "")
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []IP with condition")
}

func combin(in []NetworkingRequire) [][]NetworkingRequire {
	if len(in) == 0 {
		return nil
	}
	if len(in) == 1 {
		return [][]NetworkingRequire{in}
	}

	out := make([][]NetworkingRequire, 0, 2)

	for i := range in {
		exist := false

		for o := range out {
			if len(out[o]) > 0 && out[o][0].Networking == in[i].Networking {
				a := out[o]
				a = append(a, in[i])
				out[o] = a
				exist = true
				break
			}
		}

		if !exist {
			a := make([]NetworkingRequire, 1, 2)
			a[0] = in[i]
			out = append(out, a)
		}
	}

	return out
}

// AllocNetworking alloc IPs with UnitID in Tx
func (db dbBase) AllocNetworking(unit, engine string, requires []NetworkingRequire) ([]IP, error) {
	out := make([]IP, 0, len(requires))

	do := func(tx *sqlx.Tx) error {

		in := combin(requires)

		for _, list := range in {
			if len(list) == 0 {
				continue
			}

			key := list[0].Networking

			query := fmt.Sprintf("SELECT ip_addr,prefix,gateway,vlan_id,networking_id FROM %s WHERE networking_id=? AND enabled=? AND unit_id=? %d FOR UPDATE;", db.ipTable(), len(list))
			query = tx.Rebind(query)

			var ips []IP
			err := tx.Select(&ips, query, key, true, "")
			if err != nil {
				return errors.Wrap(err, "Tx get available IP")
			}

			for i := range list {
				ips[i].UnitID = unit
				ips[i].Engine = engine
				ips[i].Bond = list[i].Bond
				ips[i].Bandwidth = list[i].Bandwidth
			}

			out = append(out, ips...)
		}

		return db.txSetIPs(tx, out)
	}

	err := db.txFrame(do)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// txSetIPs update []IP in Tx
func (db dbBase) txSetIPs(tx *sqlx.Tx, val []IP) error {
	query := "UPDATE " + db.ipTable() + " SET unit_id=:unit_id,engine_id=:engine_id,net_dev=:net_dev,bandwidth=:bandwidth WHERE ip_addr=:ip_addr"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare update []IP")
	}

	for i := range val {
		_, err = stmt.Exec(val[i])
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

		query := "INSERT INTO " + db.ipTable() + " ( ip_addr,prefix,networking_id,unit_id,gateway,vlan_id,enabled,engine_id,net_dev,bandwidth ) VALUES ( :ip_addr,:prefix,:networking_id,:unit_id,:gateway,:vlan_id,:enabled,:engine_id,:net_dev,:bandwidth )"

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

		count := 0
		query := "SELECT COUNT(ip_addr) FROM " + db.ipTable() + " WHERE networking_id=? AND unit_id <>?"

		err := tx.Get(&count, query, networking, "")
		if err != nil {
			return errors.Wrap(err, "count networking used")
		}

		if count > 0 {
			return errors.Errorf("Networking %s has used:%d", networking, count)
		}

		_, err = tx.Exec("DELETE FROM "+db.ipTable()+" WHERE networking_id=?", networking)

		return errors.Wrap(err, "Tx delete []IP by NetworkingID")
	}

	return db.txFrame(do)
}

func (db dbBase) SetNetworkingEnable(networking string, enable bool) error {
	do := func(tx *sqlx.Tx) error {

		_, err := tx.Exec("UPDATE "+db.ipTable()+" SET enable=? WHERE networking_id=?", enable, networking)

		return errors.Wrap(err, "Tx delete []IP by NetworkingID")
	}

	return db.txFrame(do)
}

func (db dbBase) SetIPEnable(in []uint32, networking string, enable bool) error {
	do := func(tx *sqlx.Tx) error {

		stmt, err := tx.Prepare("UPDATE " + db.ipTable() + " SET enable=? WHERE ip_addr=? AND networking_id=?")
		if err != nil {
			return errors.Wrap(err, "tx prepare update []IP")
		}

		for i := range in {

			_, err = stmt.Exec(enable, in[i], networking)
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "tx prepare update []IP")
			}

		}

		stmt.Close()

		return errors.Wrap(err, "Tx update []IP")
	}

	return db.txFrame(do)
}
