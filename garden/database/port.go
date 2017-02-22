package database

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// Port is table structure,correspod with computer network port that a network applications listens on
type Port struct {
	Port      int    `db:"port"`
	Name      string `db:"name"`
	UnitID    string `db:"unit_id" json:"-"`
	UnitName  string `db:"unit_name"`
	Proto     string `db:"proto"` // tcp/udp
	Allocated bool   `db:"allocated" json:"-"`
}

func (db dbBase) portTable() string {
	return db.prefix + "_port"
}

// ListAvailablePorts returns []Port with len==num and Allocated==false
func (db dbBase) ListAvailablePorts(num int) ([]Port, error) {
	if num == 0 {
		return nil, nil
	}

	var (
		ports []Port
		query = fmt.Sprintf("SELECT port,name,unit_id,unit_name,proto,allocated FROM %s WHERE allocated=? LIMIT %d", db.portTable(), num)
	)

	err := db.Select(&ports, query, false)
	if err != nil {
		return nil, errors.Wrap(err, "list []Port")
	}

	if len(ports) != num {
		return ports, errors.Errorf("unable to get required num=%d available ports", num)
	}

	return ports, nil
}

// txSetPorts update []Port Name\UnitID\UnitName\Proto\Allocated in Tx
func (db dbBase) txSetPorts(tx *sqlx.Tx, ports []Port) error {

	query := "UPDATE " + db.portTable() + " SET name=:name,unit_id=:unit_id,unit_name=:unit_name,proto=:proto,allocated=:allocated WHERE port=:port"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare update []Port")
	}

	for i := range ports {
		_, err = stmt.Exec(&ports[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx update Port")
		}
	}

	err = stmt.Close()

	return errors.Wrap(err, "tx update []Port")
}

// ImportPort import Port from [start:end],returns number of insert ports.
func (db dbBase) ImportPort(start, end int, filter ...int) (int, error) {
	ports := make([]Port, 0, end-start+1)

loop:
	for i := start; i <= end; i++ {
		for _, val := range filter {
			if i == val {
				continue loop
			}
		}

		ports = append(ports, Port{
			Port:      i,
			Allocated: false,
		})
	}

	err := db.insertPorts(ports)
	if err == nil {
		return len(ports), nil
	}

	return 0, err
}

func (db dbBase) insertPorts(ports []Port) error {
	do := func(tx *sqlx.Tx) error {

		query := "INSERT INTO " + db.portTable() + "(port,name,unit_id,unit_name,proto,allocated) VALUES (:port,:name,:unit_id,:unit_name,:proto,:allocated)"

		stmt, err := tx.PrepareNamed(query)
		if err != nil {
			return errors.Wrap(err, "Tx prepare insert Port")
		}

		for i := range ports {
			_, err = stmt.Exec(&ports[i])
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "Tx insert Port")
			}
		}

		stmt.Close()

		return errors.Wrap(err, "Tx insert []Port")
	}

	return db.txFrame(do)
}

// ListPortsByUnit returns []Port select by UnitID or UnitName
func (db dbBase) ListPortsByUnit(nameOrID string) ([]Port, error) {
	var (
		ports []Port
		query = "SELECT port,name,unit_id,unit_name,proto,allocated FROM " + db.portTable() + " WHERE unit_id=? OR unit_name=?"
	)

	err := db.Select(&ports, query, nameOrID, nameOrID)

	return ports, errors.Wrap(err, "list []Port")
}

// ListPorts returns []Port by condition start\end\limit.
func (db dbBase) ListPorts(start, end, limit int) ([]Port, error) {
	var (
		err   error
		ports []Port
	)

	switch {
	case start == 0 && end == 0:
		query := fmt.Sprintf("SELECT port,name,unit_id,unit_name,proto,allocated FROM %s LIMIT %d", db.portTable(), limit)
		err = db.Select(&ports, query)

	case start > 0 && end > 0:
		query := fmt.Sprintf("SELECT port,name,unit_id,unit_name,proto,allocated FROM %s WHERE port>=? AND port<=? LIMIT %d", db.portTable(), limit)
		err = db.Select(&ports, query, start, end)

	case end == 0:
		query := fmt.Sprintf("SELECT port,name,unit_id,unit_name,proto,allocated FROM %s WHERE port>=? LIMIT %d", db.portTable(), limit)
		err = db.Select(&ports, query, start)

	case start == 0:
		query := fmt.Sprintf("SELECT port,name,unit_id,unit_name,proto,allocated FROM %s WHERE port<=? LIMIT %d", db.portTable(), limit)
		err = db.Select(&ports, query, end)

	default:
		return nil, errors.Errorf("illegal input,start=%d end=%d limit=%d", start, end, limit)
	}

	return ports, errors.Wrap(err, "list []Port")
}

// DelPort delete by port,only if Port.Allocated==allocated
func (db dbBase) DelPort(port int, allocated bool) error {
	using := false

	err := db.Get(&using, "SELECT allocated FROM "+db.portTable()+" WHERE port=?", port)
	if err != nil {
		return errors.Wrap(err, "get Port.Allocated")
	}

	if using != allocated {
		return errors.Errorf("Port %d (%t!= %t),cannot be removed", port, using, allocated)
	}

	_, err = db.Exec("DELETE FROM "+db.portTable()+" WHERE port=? AND allocated=?", port, allocated)

	return errors.Wrap(err, "delete Port")
}
