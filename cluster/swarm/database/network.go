package database

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/jmoiron/sqlx"
)

type IP struct {
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       uint32 `db:"prefix"`
	NetworkingID string `db:"networking_id"`
	UnitID       string `db:"unit_id"`
	Allocated    bool   `db:"allocated"`
}

func (ip IP) TableName() string {
	return "tb_ip"
}

type Networking struct {
	ID         string `db:"id"`
	Networking string `db:"networking"`
	Type       string `db:"type"`
	Gateway    string `db"gateway"`
	Enabled    bool   `db:"enabled"`
}

func (net Networking) TableName() string {
	return "tb_networking"
}

type Port struct {
	Port      int    `db:"port"` // auto increment
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

func SelectAvailablePorts(num int) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SELECT * FROM tb_port WHERE allocated=? LIMIT %d", num)

	rows, err := db.Queryx(query, false)
	if err != nil {
		return nil, err
	}

	var ports []Port

	err = rows.StructScan(&ports)
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
func TxImportPort(start, end int, filter ...int) error {
	ports := make([]Port, 0, end-start)

loop:
	for i := start; i <= end; i++ {
		for j := range filter {
			if i == j {
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
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = TxInsertPorts(tx, ports)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func TxInsertPorts(tx *sqlx.Tx, ports []Port) error {
	query := "INSERT INTO tb_port (port,name,unit_id,allocated) VALUES (:port,:name,:unit_id,:allocated)"

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
	err = db.QueryRowx("SELECT * From tb_port WHERE unit_id=?", id).StructScan(&ports)
	if err != nil {
		return nil, err
	}

	return ports, nil
}

func InsertNetworking(id, addr, gateway, typ string, prefix, num int) error {
	net := Networking{
		ID:         id,
		Networking: addr,
		Type:       typ,
		Gateway:    gateway,
		Enabled:    true,
	}

	ips := make([]IP, num)
	addrU32 := IPToUint32(net.Networking)
	prefixU32 := uint32(prefix)

	for i := range ips {
		ips[i] = IP{
			IPAddr:       addrU32,
			Prefix:       prefixU32,
			NetworkingID: net.ID,
			Allocated:    false,
		}

		addrU32++
	}

	// insert to database

	return insertNetworking(net, ips)
}

func insertNetworking(net Networking, ips []IP) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
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

type IPStatus struct {
	UnitID    string
	IP        uint32
	Prefix    int
	Allocated bool
}

func TxUpdateMultiIPStatue(tx *sqlx.Tx, val []IPStatus) error {
	query := "UPDATE tb_ip SET unit_id=:unit_id,allocated=:allocated WHERE ip=:ip AND prefix=:prefix"

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

func IPToUint32(ip string) uint32 {
	addr := net.ParseIP(ip)
	if addr == nil {
		return 0
	}
	return binary.BigEndian.Uint32(addr.To4())
}

func Uint32ToIP(cidr uint32) net.IP {
	addr := make([]byte, 4)
	binary.BigEndian.PutUint32(addr, cidr)
	return net.IP(addr)
}
