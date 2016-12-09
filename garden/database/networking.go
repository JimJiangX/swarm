package database

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type NetworkingOrmer interface {
	// IP
	ListIPByNetworking(networkingID string) ([]IP, error)
	ListIPByUnitID(unit string) ([]IP, error)
	ListIPWithCondition(networking string, allocation bool, num int) ([]IP, error)

	AllocIPByNetworking(id, unit string) (IP, error)
	UpdateIPs(tx *sqlx.Tx, val []IP) error

	// Port
	ImportPort(start, end int, filter ...int) (int, error)
	UpdatePorts(ports []Port) error

	ListAvailablePorts(num int) ([]Port, error)
	ListPortsByUnit(nameOrID string) ([]Port, error)
	ListPorts(start, end, limit int) ([]Port, error)

	DeletePort(port int, allocated bool) error

	// Networking
	InsertNetworking(Networking, []IP) error
	UpdateNetworkingStatus(ID string, enable bool) error

	GetNetworkingByID(ID string) (Networking, int, error)
	ListNetworking() ([]Networking, error)
	ListNetworkingByType(_type string) ([]Networking, error)

	DeleteNetworking(ID string) error
	IsNetwrokingUsed(networking string) (bool, error)
}

// IP is table structure, associate to Networking
type IP struct {
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       int    `db:"prefix"`
	NetworkingID string `db:"networking_id"`
	UnitID       string `db:"unit_id"`
	Allocated    bool   `db:"allocated"`
}

func (db dbBase) ipTable() string {
	return db.prefix + "_ip"
}

// ListIPByNetworking returns []IP select by NetworkingID
func (db dbBase) ListIPByNetworking(networkingID string) ([]IP, error) {
	var (
		list  []IP
		query = "SELECT ip_addr,prefix,networking_id,unit_id,allocated FROM " + db.ipTable() + " WHERE networking_id=?"
	)

	err := db.Select(&list, query, networkingID)

	return list, errors.Wrap(err, "list []IP by networking")
}

// ListIPByUnitID returns []IP select by UnitID
func (db dbBase) ListIPByUnitID(unit string) ([]IP, error) {
	var (
		out   []IP
		query = "SELECT ip_addr,prefix,networking_id,unit_id,allocated from " + db.ipTable() + " WHERE unit_id=?"
	)

	err := db.Select(&out, query, unit)

	return out, errors.Wrap(err, "list []IP by UnitID")
}

// ListIPWithCondition returns []IP select by NetworkingID and Allocated==allocated
func (db dbBase) ListIPWithCondition(networking string, allocation bool, num int) ([]IP, error) {
	var (
		out   []IP
		query = fmt.Sprintf("SELECT ip_addr,prefix,networking_id,unit_id,allocated from %s WHERE networking_id=? AND allocated=? LIMIT %d", db.ipTable(), num)
	)

	err := db.Select(&out, query, networking, allocation)

	return out, errors.Wrap(err, "list []IP with condition")
}

// TxAllocIPByNetworking update IP UnitID in Tx
func (db dbBase) AllocIPByNetworking(id, unit string) (IP, error) {
	out := &IP{}

	do := func(tx *sqlx.Tx) error {

		query := "SELECT ip_addr,prefix FROM " + db.ipTable() + " WHERE networking_id=? AND allocated=? LIMIT 1 FOR UPDATE;"
		query = tx.Rebind(query)

		err := tx.Get(out, query, id, false)
		if err != nil {
			return errors.Wrap(err, "Tx get available IP")
		}

		out.Allocated = true
		out.UnitID = unit

		query = "UPDATE " + db.ipTable() + " SET allocated=:allocated,unit_id=:unit_id WHERE ip_addr=:ip_addr AND prefix=:prefix"

		_, err = tx.NamedExec(query, *out)

		return errors.Wrap(err, "tx update IP")
	}

	err := db.txFrame(do)

	return *out, errors.Wrap(err, "tx update IP")
}

// TxUpdateIPs update []IP in Tx
func (db dbBase) UpdateIPs(tx *sqlx.Tx, val []IP) error {
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

// TxUpdatePorts update []Port Name\UnitID\UnitName\Proto\Allocated in Tx
func (db dbBase) txUpdatePorts(tx *sqlx.Tx, ports []Port) error {

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

// TxImportPort import Port from [start:end],returns number of insert ports.
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

// DeletePort delete by port,only if Port.Allocated==allocated
func (db dbBase) DeletePort(port int, allocated bool) error {
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

// Networking is table structure,a goroup of IP Address
type Networking struct {
	ID      string `db:"id"`
	Type    string `db:"type"`
	Gateway string `db:"gateway"`
	Enabled bool   `db:"enabled"`
}

func (db dbBase) networkingTable() string {
	return db.prefix + "_networking"
}

// GetNetworkingByID returns Networking and IP.Prefix select by Networking ID.
func (db dbBase) GetNetworkingByID(ID string) (Networking, int, error) {
	net := Networking{}

	err := db.Get(&net, "SELECT id,type,gateway,enabled FROM "+db.networkingTable()+" WHERE id=?", ID)
	if err != nil {
		return net, 0, errors.Wrap(err, "get Networking by ID:"+ID)
	}

	prefix := 0
	err = db.Get(&prefix, "SELECT prefix FROM "+db.ipTable()+" WHERE networking_id=? LIMIT 1", ID)
	if err != nil {
		return net, 0, errors.Wrap(err, "get IP.Prefix")
	}

	return net, prefix, nil
}

// ListNetworkingByType returns []Netwroking select by Networking.Type
func (db dbBase) ListNetworkingByType(_type string) ([]Networking, error) {
	var (
		list  []Networking
		query = "SELECT id,type,gateway,enabled FROM " + db.networkingTable() + " WHERE type=?"
	)

	err := db.Select(&list, query, _type)

	return list, errors.Wrap(err, "list []Networking by Type")
}

// ListNetworking returns all []Networking.
func (db dbBase) ListNetworking() ([]Networking, error) {
	var (
		list  []Networking
		query = "SELECT id,type,gateway,enabled FROM " + db.networkingTable() + ""
	)

	err := db.Select(&list, query)

	return list, errors.Wrap(err, "list all []Networking")
}

// UpdateNetworkingStatus upate Networking Enabled by ID
func (db dbBase) UpdateNetworkingStatus(ID string, enable bool) error {

	query := "UPDATE " + db.networkingTable() + " SET enabled=? WHERE id=?"

	_, err := db.Exec(query, enable, ID)

	return errors.Wrap(err, "update Networking.Enabled")
}

func (db dbBase) InsertNetworking(net Networking, ips []IP) error {
	do := func(tx *sqlx.Tx) error {

		query := "INSERT INTO " + db.ipTable() + " (ip_addr,prefix,networking_id,unit_id,allocated) VALUES (:ip_addr,:prefix,:networking_id,:unit_id,:allocated)"

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

		query = "INSERT INTO " + db.networkingTable() + " (id,type,gateway,enabled) VALUES (:id,:type,:gateway,:enabled)"

		_, err = tx.NamedExec(query, &net)

		return errors.Wrap(err, "Tx insert Networking")
	}

	return db.txFrame(do)
}

// TxDeleteNetworking delete Networking and []IP in Tx
func (db dbBase) DeleteNetworking(ID string) error {
	do := func(tx *sqlx.Tx) error {

		_, err := tx.Exec("DELETE FROM "+db.networkingTable()+" WHERE id=?", ID)
		if err != nil {
			return errors.Wrap(err, "Tx delete Networking by ID")
		}

		_, err = tx.Exec("DELETE FROM "+db.ipTable()+" WHERE networking_id=?", ID)

		return errors.Wrap(err, "Tx delete []IP by NetworkingID")
	}

	return db.txFrame(do)
}

// IsNetwrokingUsed returns true on below conditions:
// one more IP belongs to networking has allocated
// networking has used in Cluster
func (db dbBase) IsNetwrokingUsed(networking string) (bool, error) {
	var (
		count        = 0
		queryIP      = "SELECT COUNT(ip_addr) from " + db.ipTable() + " WHERE networking_id=? AND allocated=?"
		queryCluster = "SELECT COUNT(id) from " + db.clusterTable() + " WHERE networking_id=?"
	)

	err := db.Get(&count, queryIP, networking, true)
	if err != nil {
		return false, errors.Wrap(err, "count []IP by NetworkingID")
	}

	if count > 0 {
		return true, nil
	}

	err = db.Get(&count, queryCluster, networking)
	if err != nil {
		return false, errors.Wrap(err, "count []Cluster by NetworkingID")
	}

	return count > 0, nil
}
