package database

import (
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

func NewPort(port int, name, unitID, unitName, proto string, allocated bool) Port {
	return Port{
		Port:      port,
		Name:      name,
		UnitID:    unitID,
		UnitName:  unitName,
		Proto:     proto,
		Allocated: allocated,
	}
}

func delMultiPorts(tx *sqlx.Tx, ports []Port) error {
	query := "DELETE FROM tb_port WHERE port=?"

	stmt, err := tx.Preparex(query)
	if err != nil {
		return errors.Wrap(err, "Prepare Delete Port")
	}

	for i := range ports {
		_, err := stmt.Exec(ports[i].Port)
		if err != nil {
			// return err
		}
	}

	stmt.Close()

	return nil
}

func TestPort(t *testing.T) {
	ports := []Port{
		NewPort(1, "port1", "unit1", "unitName1", "tcp", true),
		NewPort(2, "port2", "unit2", "unitName2", "tcp", false),
		NewPort(3, "port3", "unit3", "unitName3", "udp", true),
		NewPort(4, "port4", "unit4", "unitName4", "udp", false),
	}

	defer func() {
		tx, err := GetTX()
		if err != nil {
			t.Fatal(err)
		}
		err = delMultiPorts(tx, ports)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit()
		if err != nil {
			t.Fatal(err)
		}
	}()

	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = txInsertPorts(tx, ports)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	p1, err := ListAvailablePorts(4)
	if err != nil {
		t.Log(err)
	}
	if len(p1) != 2 {
		t.Fatal("Available Ports should be 2", p1)
	}

	_p3 := NewPort(3, "port3b", "unit3b", "unitName3b", "udp", false)
	p3 := []Port{_p3}

	tx, err = GetTX()
	err = TxUpdatePorts(tx, p3)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	p4, err := ListAvailablePorts(4)
	if err != nil {
		t.Log(err)
	}
	if len(p4) != 3 {
		t.Fatal("Available Ports should be 3")
	}

	n, err := TxImportPort(5, 10, 7, 8)
	defer func() {
		tx, err := GetTX()
		if err != nil {
			t.Fatal(err)
		}
		err = delMultiPorts(
			tx,
			[]Port{
				NewPort(5, "", "", "", "", true),
				NewPort(6, "", "", "", "", true),
				NewPort(9, "", "", "", "", true),
				NewPort(10, "", "", "", "", true),
			})
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit()
		if err != nil {
			t.Fatal(err)
		}
	}()

	p5, err := ListAvailablePorts(10)
	if err != nil {
		t.Log(err)
	}
	if len(p5) != 3+n {
		t.Fatal("Available Ports should be 8")
	}

	p6, err := ListPortsByUnit("unit1")
	if err != nil {
		t.Fatal(err)
	}
	if len(p6) != 1 {
		t.Fatal("unit1 Ports should be 1")
	}
}

func TestNetwork(t *testing.T) {
	networking, _, err := TxInsertNetworking("192.168.2.14", "192.168.2.144", "255.255.255.0", "tcp", 24)
	if err != nil {
		t.Fatal(err)
	}

	defer TxDeleteNetworking(networking.ID)

	err = UpdateNetworkingStatus(networking.ID, false)
	if err != nil {
		t.Fatal(err)
	}
}

func randomIP() string {
	rand.Seed(time.Now().Unix())
	ip1 := rand.Int31n(255)
	rand.Seed(int64(ip1))
	ip2 := rand.Int31n(255)
	rand.Seed(int64(ip2))
	ip3 := rand.Int31n(255)
	rand.Seed(int64(ip3))
	ip4 := rand.Int31n(255)
	ip := net.IPv4(byte(ip1), byte(ip2), byte(ip3), byte(ip4))
	return ip.String()
}
