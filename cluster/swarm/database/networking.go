package database

import (
	"fmt"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
)

type IP struct {
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       int    `db:"prefix"`
	NetworkingID string `db:"networking_id"`
	UnitID       string `db:"unit_id"`
	Allocated    bool   `db:"allocated"`
}

func (ip IP) TableName() string {
	return "tb_ip"
}

type Networking struct {
	ID      string `db:"id"`
	Type    string `db:"type"`
	Gateway string `db"gateway"`
	Enabled bool   `db:"enabled"`
}

func (net Networking) TableName() string {
	return "tb_networking"
}

type Port struct {
	Port      int    `db:"port"`
	Name      string `db:"name"`
	UnitID    string `db:"unit_id" json:"-"`
	Proto     string `db:"proto"` // tcp/udp
	Allocated bool   `db:"allocated" json:"-"`
}

func (port Port) TableName() string {
	return "tb_port"
}

func NewPort(port int, name, unit, proto string, allocated bool) Port {
	return Port{
		Port:      port,
		Name:      name,
		UnitID:    unit,
		Proto:     proto,
		Allocated: allocated,
	}
}

func SetPortAllocated(port int, allocated bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_port SET allocated=? WHERE port=?", allocated, port)

	return err
}

func SelectAvailablePorts(num int) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	ports := make([]Port, 0, num)
	query := fmt.Sprintf("SELECT * FROM tb_port WHERE allocated=? LIMIT %d", num)

	err = db.Select(&ports, query, false)
	if err != nil {
		return nil, err
	}

	return ports, nil
}

func TxUpdatePorts(tx *sqlx.Tx, ports []Port) error {
	query := "UPDATE tb_port SET name=:name,unit_id=:unit_id,proto=:proto,allocated=:allocated WHERE port=:port"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}

	for i := range ports {
		_, err = stmt.Exec(&ports[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// TxImportPort import Port from start to end(includ end)
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

	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	err = TxInsertPorts(tx, ports)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return len(ports), nil
}

func TxInsertPorts(tx *sqlx.Tx, ports []Port) error {
	query := "INSERT INTO tb_port (port,name,unit_id,proto,allocated) VALUES (:port,:name,:unit_id,:proto,:allocated)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}

	for i := range ports {
		_, err = stmt.Exec(&ports[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func DelMultiPorts(tx *sqlx.Tx, ports []Port) error {
	query := "DELETE FROM tb_port WHERE port=?"

	stmt, err := tx.Preparex(query)
	if err != nil {
		return err
	}

	for i := range ports {
		_, err := stmt.Exec(ports[i].Port)
		if err != nil {
			// return err
		}
	}

	return nil
}

func GetPortsByUnit(id string) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var ports []Port
	err = db.Select(&ports, "SELECT * FROM tb_port WHERE unit_id=?", id)
	if err != nil {
		return nil, err
	}

	return ports, nil
}

func GetNetworkingByID(id string) (Networking, int, error) {
	db, err := GetDB(true)
	if err != nil {
		return Networking{}, 0, err
	}

	net := Networking{}

	err = db.Get(&net, "SELECT * FROM tb_networking WHERE id=?", id)
	if err != nil {
		return net, 0, err
	}

	prefix := 0
	err = db.Get(&prefix, "SELECT prefix FROM tb_ip WHERE networking_id=? LIMIT 1", id)
	if err != nil {
		return Networking{}, 0, err
	}

	return net, prefix, nil
}

func ListNetworkingByType(typ string) ([]Networking, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	list := make([]Networking, 0, 5)

	err = db.Select(&list, "SELECT * FROM tb_networking WHERE type=?", typ)
	if err != nil {
		return nil, err
	}

	return list, nil
}

func TxInsertNetworking(start, end, gateway, typ string, prefix int) (Networking, []IP, error) {
	startU32 := utils.IPToUint32(start)
	endU32 := utils.IPToUint32(end)
	if move := uint(32 - prefix); (startU32 >> move) != (endU32 >> move) {
		return Networking{}, nil, fmt.Errorf("%s-%s is different network segments", start, end)
	}
	if startU32 > endU32 {
		startU32, endU32 = endU32, startU32
	}
	net := Networking{
		ID:      utils.Generate64UUID(),
		Type:    typ,
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

		fmt.Println(i, startU32, utils.Uint32ToIP(startU32).String())

		startU32++
	}

	// insert to database
	err := insertNetworking(net, ips)
	if err != nil {
		return Networking{}, nil, err
	}

	return net, ips, nil
}

func UpdateNetworkingStatus(id string, enable bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE tb_networking SET enabled=? WHERE id=?", enable, id)

	return err
}

func insertNetworking(net Networking, ips []IP) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := "INSERT INTO tb_networking (id,networking,type,gateway,enabled) VALUES (:id,:networking,:type,:gateway,:enabled)"
	_, err = tx.NamedExec(query, &net)
	if err != nil {
		return err
	}

	ipQuery := "INSERT INTO tb_ip (ip_addr,prefix,networking_id,unit_id,allocated) VALUES (:ip_addr,:prefix,:networking_id,:unit_id,:allocated)"
	stmt, err := tx.PrepareNamed(ipQuery)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range ips {
		_, err = stmt.Exec(&ips[i])
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func TxDeleteNetworking(id string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM tb_networking WHERE id=?", id)
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM tb_ip WHERE networking_id=?", id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func CountIPByNetwrokingAndStatus(networking string, allocation bool) (int, error) {
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	count := 0
	err = db.Get(&count, "SELECT COUNT(*) from tb_ip WHERE networking_id=? AND allocated=?", networking, allocation)

	return count, err
}

func GetMultiIPByNetworking(networking string, allocation bool, num int) ([]IP, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	out := make([]IP, num)
	query := fmt.Sprintf("SELECT * from tb_ip WHERE networking_id=? AND allocated=? LIMIT %d", num)

	err = db.Select(&out, query, networking, allocation)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func TXAllocIPByNetworking(id, unit string) (IP, error) {
	tx, err := GetTX()
	if err != nil {
		return IP{}, err
	}
	defer tx.Rollback()

	out := IP{}
	query := tx.Rebind(fmt.Sprintf("SELECT * FROM tb_ip WHERE networking_id=? AND allocated=? LIMIT 1 FOR UPDATE;"))

	err = tx.Get(&out, query, id, false)
	if err != nil {
		return out, err
	}

	out.Allocated = true
	out.UnitID = unit

	query = "UPDATE tb_ip SET allocated=:allocated,unit_id=:unit_id WHERE ip_addr=:ip_addr AND prefix=:prefix"

	_, err = tx.NamedExec(query, out)
	if err != nil {
		return out, err
	}

	if err = tx.Commit(); err != nil {
		return out, err
	}

	return out, nil
}

func TxUpdateMultiIPValue(tx *sqlx.Tx, val []IP) error {
	query := "UPDATE tb_ip SET unit_id=:unit_id,allocated=:allocated WHERE ip_addr=:ip_addr AND prefix=:prefix"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range val {
		_, err = stmt.Exec(&val[i])
		if err != nil {
			return err
		}
	}

	return nil
}