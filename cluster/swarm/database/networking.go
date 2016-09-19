package database

import (
	"fmt"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertIPQuery = "INSERT INTO tb_ip (ip_addr,prefix,networking_id,unit_id,allocated) VALUES (:ip_addr,:prefix,:networking_id,:unit_id,:allocated)"

// IP is table tb_ip structure, associate to Networking
type IP struct {
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       int    `db:"prefix"`
	NetworkingID string `db:"networking_id"`
	UnitID       string `db:"unit_id"`
	Allocated    bool   `db:"allocated"`
}

func (ip IP) tableName() string {
	return "tb_ip"
}

const insertNetworkingQuery = "INSERT INTO tb_networking (id,type,gateway,enabled) VALUES (:id,:type,:gateway,:enabled)"

// Networking is table tb_networking structure,a goroup of IP Address
type Networking struct {
	ID      string `db:"id"`
	Type    string `db:"type"`
	Gateway string `db:"gateway"`
	Enabled bool   `db:"enabled"`
}

func (net Networking) tableName() string {
	return "tb_networking"
}

const insertPortQuery = "INSERT INTO tb_port (port,name,unit_id,unit_name,proto,allocated) VALUES (:port,:name,:unit_id,:unit_name,:proto,:allocated)"

// Port is table tb_port structure,correspod with computer network port that a network applications listens on
type Port struct {
	Port      int    `db:"port"`
	Name      string `db:"name"`
	UnitID    string `db:"unit_id" json:"-"`
	UnitName  string `db:"unit_name"`
	Proto     string `db:"proto"` // tcp/udp
	Allocated bool   `db:"allocated" json:"-"`
}

func (port Port) tableName() string {
	return "tb_port"
}

// DeletePort delete by port,only if Port.Allocated==allocated
func DeletePort(port int, allocated bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	using := false
	err = db.Get(&using, "SELECT allocated FROM tb_port WHERE port=?", port)
	if err != nil {
		return errors.Wrap(err, "get Port.Allocated")
	}
	if using != allocated {
		return errors.Errorf("Port %d (%t!= %t),cannot be removed", port, using, allocated)
	}

	_, err = db.Exec("DELETE FROM tb_port WHERE port=? AND allocated=?", port, allocated)

	return errors.Wrap(err, "delete Port")
}

// ListAvailablePorts returns []Port with len==num and Allocated==false
func ListAvailablePorts(num int) ([]Port, error) {
	if num == 0 {
		return nil, nil
	}

	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var (
		ports []Port
		query = fmt.Sprintf("SELECT * FROM tb_port WHERE allocated=? LIMIT %d", num)
	)

	err = db.Select(&ports, query, false)
	if err != nil {
		db, err = GetDB(true)
		if err != nil {
			return nil, err
		}

		err = db.Select(&ports, query, false)
		if err != nil {
			return nil, errors.Wrap(err, "list []Port")
		}
	}

	if len(ports) != num {
		return ports, errors.Errorf("unable to get required num=%d available ports", num)
	}

	return ports, nil
}

// TxUpdatePorts update []Port Name\UnitID\UnitName\Proto\Allocated in Tx
func TxUpdatePorts(tx *sqlx.Tx, ports []Port) error {
	const query = "UPDATE tb_port SET name=:name,unit_id=:unit_id,unit_name=:unit_name,proto=:proto,allocated=:allocated WHERE port=:port"

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

	return errors.Wrap(err, "Tx update []Port")
}

// TxImportPort import Port from [start:end],returns number of insert ports.
func TxImportPort(start, end int, filter ...int) (int, error) {
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

	tx, err := GetTX()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	err = txInsertPorts(tx, ports)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, errors.Wrap(err, "Tx insert []Port")
	}

	return len(ports), nil
}

func txInsertPorts(tx *sqlx.Tx, ports []Port) error {
	stmt, err := tx.PrepareNamed(insertPortQuery)
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

	err = stmt.Close()

	return errors.Wrap(err, "Tx insert []Port")
}

// ListPortsByUnit returns []Port select by UnitID or UnitName
func ListPortsByUnit(nameOrID string) ([]Port, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var ports []Port
	const query = "SELECT * FROM tb_port WHERE unit_id=? OR unit_name=?"

	err = db.Select(&ports, query, nameOrID, nameOrID)
	if err == nil {
		return ports, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&ports, query, nameOrID, nameOrID)

	return ports, errors.Wrap(err, "list []Port")
}

// ListPorts returns []Port by condition start\end\limit.
func ListPorts(start, end, limit int) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	ports := make([]Port, 0, limit)

	switch {
	case start == 0 && end == 0:
		query := fmt.Sprintf("SELECT * FROM tb_port LIMIT %d", limit)
		err = db.Select(&ports, query)

	case start > 0 && end > 0:
		query := fmt.Sprintf("SELECT * FROM tb_port WHERE port>=? AND port<=? LIMIT %d", limit)
		err = db.Select(&ports, query, start, end)

	case end == 0:
		query := fmt.Sprintf("SELECT * FROM tb_port WHERE port>=? LIMIT %d", limit)
		err = db.Select(&ports, query, start)

	case start == 0:
		query := fmt.Sprintf("SELECT * FROM tb_port WHERE port<=? LIMIT %d", limit)
		err = db.Select(&ports, query, end)

	default:
		return nil, errors.Errorf("illegal input,start=%d end=%d limit=%d", start, end, limit)
	}

	return ports, errors.Wrap(err, "list []Port")
}

// GetNetworkingByID returns Networking and IP.Prefix select by Networking ID.
func GetNetworkingByID(ID string) (Networking, int, error) {
	db, err := GetDB(true)
	if err != nil {
		return Networking{}, 0, err
	}

	net := Networking{}

	err = db.Get(&net, "SELECT * FROM tb_networking WHERE id=?", ID)
	if err != nil {
		return net, 0, errors.Wrap(err, "get Networking by ID:"+ID)
	}

	prefix := 0
	err = db.Get(&prefix, "SELECT prefix FROM tb_ip WHERE networking_id=? LIMIT 1", ID)
	if err != nil {
		return net, 0, errors.Wrap(err, "get IP.Prefix")
	}

	return net, prefix, nil
}

// ListIPByUnitID returns []IP select by UnitID
func ListIPByUnitID(unit string) ([]IP, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var out []IP
	const query = "SELECT * from tb_ip WHERE unit_id=?"

	err = db.Select(&out, query, unit)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, unit)

	return out, errors.Wrap(err, "list []IP by UnitID")
}

// ListNetworkingByType returns []Netwroking select by Networking.Type
func ListNetworkingByType(_type string) ([]Networking, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []Networking
	const query = "SELECT * FROM tb_networking WHERE type=?"

	err = db.Select(&list, query, _type)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, _type)

	return list, errors.Wrap(err, "list []Networking by Type")
}

// ListNetworking returns all []Networking.
func ListNetworking() ([]Networking, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []Networking
	const query = "SELECT * FROM tb_networking"

	err = db.Select(&list, query)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query)

	return list, errors.Wrap(err, "list all []Networking")
}

// ListIPByNetworking returns []IP select by NetworkingID
func ListIPByNetworking(networkingID string) ([]IP, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []IP
	const query = "SELECT * FROM tb_ip WHERE networking_id=?"

	err = db.Select(&list, query, networkingID)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, networkingID)

	return list, errors.Wrap(err, "list []IP by networking")
}

// TxInsertNetworking insert Networking in Tx and []IP,and returns them
func TxInsertNetworking(start, end, gateway, _type string, prefix int) (Networking, []IP, error) {
	startU32 := utils.IPToUint32(start)
	endU32 := utils.IPToUint32(end)
	if move := uint(32 - prefix); (startU32 >> move) != (endU32 >> move) {
		return Networking{}, nil, errors.Errorf("%s-%s is different network segments", start, end)
	}
	if startU32 > endU32 {
		startU32, endU32 = endU32, startU32
	}
	net := Networking{
		ID:      utils.Generate64UUID(),
		Type:    _type,
		Gateway: gateway,
		Enabled: true,
	}

	num := int(endU32 - startU32 + 1)
	ips := make([]IP, num)
	for i := range ips {
		ips[i] = IP{
			IPAddr:       startU32,
			Prefix:       prefix,
			NetworkingID: net.ID,
			Allocated:    false,
		}

		// fmt.Println(i, startU32, utils.Uint32ToIP(startU32).String())

		startU32++
	}

	// insert to database
	err := txInsertNetworking(net, ips)
	if err != nil {
		return Networking{}, nil, err
	}

	return net, ips, nil
}

// UpdateNetworkingStatus upate Networking Enabled by ID
func UpdateNetworkingStatus(ID string, enable bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_networking SET enabled=? WHERE id=?"

	_, err = db.Exec(query, enable, ID)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, enable, ID)

	return errors.Wrap(err, "update Networking.Enabled")
}

func txInsertNetworking(net Networking, ips []IP) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareNamed(insertIPQuery)
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

	_, err = tx.NamedExec(insertNetworkingQuery, &net)
	if err != nil {
		return errors.Wrap(err, "Tx insert Networking")
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx insert []IP and Networking")
}

// TxDeleteNetworking delete Networking and []IP in Tx
func TxDeleteNetworking(ID string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM tb_networking WHERE id=?", ID)
	if err != nil {
		return errors.Wrap(err, "Tx delete Networking by ID")
	}

	_, err = tx.Exec("DELETE FROM tb_ip WHERE networking_id=?", ID)
	if err != nil {
		return errors.Wrap(err, "Tx delete []IP by NetworkingID")
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx delete Networking and []IP by ID")
}

// IsNetwrokingUsed returns true on below conditions:
// one more IP belongs to networking has allocated
// networking has used in Cluster
func IsNetwrokingUsed(networking string) (bool, error) {
	db, err := GetDB(true)
	if err != nil {
		return false, err
	}

	count := 0
	const (
		queryIP      = "SELECT COUNT(*) from tb_ip WHERE networking_id=? AND allocated=?"
		queryCluster = "SELECT COUNT(*) from tb_cluster WHERE networking_id=?"
	)

	err = db.Get(&count, queryIP, networking, true)
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

// ListIPWithCondition returns []IP select by NetworkingID and Allocated==allocated
func ListIPWithCondition(networking string, allocation bool, num int) ([]IP, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var out []IP
	query := fmt.Sprintf("SELECT * from tb_ip WHERE networking_id=? AND allocated=? LIMIT %d", num)

	err = db.Select(&out, query, networking, allocation)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, networking, allocation)

	return nil, errors.Wrap(err, "list []IP with condition")
}

// TxAllocIPByNetworking update IP UnitID in Tx
func TxAllocIPByNetworking(id, unit string) (IP, error) {
	tx, err := GetTX()
	if err != nil {
		return IP{}, err
	}
	defer tx.Rollback()

	out := IP{}
	query := tx.Rebind(fmt.Sprintf("SELECT * FROM tb_ip WHERE networking_id=? AND allocated=? LIMIT 1 FOR UPDATE;"))

	err = tx.Get(&out, query, id, false)
	if err != nil {
		return out, errors.Wrap(err, "Tx get available IP")
	}

	out.Allocated = true
	out.UnitID = unit

	query = "UPDATE tb_ip SET allocated=:allocated,unit_id=:unit_id WHERE ip_addr=:ip_addr AND prefix=:prefix"

	_, err = tx.NamedExec(query, out)
	if err != nil {
		return out, errors.Wrap(err, "tx update IP")
	}

	err = tx.Commit()

	return out, errors.Wrap(err, "tx update IP")
}

// TxUpdateIPs update []IP in Tx
func TxUpdateIPs(tx *sqlx.Tx, val []IP) error {
	const query = "UPDATE tb_ip SET unit_id=:unit_id,allocated=:allocated WHERE ip_addr=:ip_addr AND prefix=:prefix"

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
