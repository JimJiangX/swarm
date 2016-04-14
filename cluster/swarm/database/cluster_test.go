package database

import (
	"testing"
	"time"
)

func TestCluster(t *testing.T) {
	clusters := []Cluster{
		NewCluster("cluster1", "upsql", "local", "", "dc1", true, 256, 0.8),
		NewCluster("cluster2", "upsql", "ssd", "", "dc1", true, 100000000, 1.44),
		NewCluster("cluster3", "proxy", "local", "", "dc1", true, 100000000, 1.44),
		NewCluster("cluster4", "proxy", "local", "", "dc3", false, 100000000, 1.44),
	}
	wrong := []Cluster{
		NewCluster("cluster2", "nomal", "ssd", "", "dc1", false, 1000000000000, 1.44),
		NewCluster("cluster3", "proxy", "local", "", "dc1", false, 100000000, 1.44),
		NewCluster("cluster4", "proxy", "local", "", "dc3", false, 100000000, 1.44),
	}

	for i := range clusters {
		if err := clusters[i].Insert(); err != nil {
			t.Fatal(clusters[i].Name, err)
		}

		defer func(id string) {
			if err := DeleteCluster(id); err != nil {
				t.Error(id, err)
			}
		}(clusters[i].ID)
	}

	for i := range wrong {
		if err := wrong[i].Insert(); err == nil {
			t.Fatal("Name should not allowed Duplicate", wrong[i].Name)
		}
	}

	cl1, err := GetCluster("cluster3")
	if err != nil {
		t.Fatal(err)
	}
	if cl1.ID != clusters[2].ID {
		t.Fatalf("Unexpected,%s != %s", cl1.ID, clusters[2].ID)
	}

	_, err = GetCluster("cluster5")
	if err == nil {
		t.Fatal(err)
	}

	cl2 := Cluster{ID: clusters[2].ID}
	if err := cl2.UpdateStatus(false); err != nil {
		t.Fatal(err)
	}
	cl3, err := GetCluster(cl2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cl2.Enabled != cl3.Enabled {
		t.Fatalf("Unexpected,%t != %t", cl2.Enabled, cl3.Enabled)
	}

	for i := range clusters {
		if err := DeleteCluster(wrong[i].ID); err != nil {
			t.Fatal(wrong[i].Name, err)
		}
	}
}

func TestNode(t *testing.T) {
	list := []Node{
		NewNode("node1", "cluster1", "192.168.2.10:8888", 100, 1000, time.Now(), time.Time{}),
		NewNode("node2", "cluster2", "192.168.2.144:8888", 1000, 1000, time.Time{}, time.Now()),
		NewNode("node3", "cluster1", "192.168.2.134:8888", 100, 0, time.Time{}, time.Time{}),
		NewNode("node4", "cluster2", "192.168.2.13:8888", 0, 1000, time.Now(), time.Now()),
		NewNode("node5", "cluster3", "192.168.2.10:8888", 100, 100100, time.Now(), time.Time{}),
	}

	for i := range list {
		err := list[i].Insert()
		if err != nil {
			if list[i].Name == "node5" {
				t.Skip("Expected")
			}

			t.Fatal(list[i].Name, err)
		}

		defer func(id string) {
			if err := DeleteNode(id); err != nil {
				t.Error(id, err)
			}
		}(list[i].ID)
	}

	node1, err := GetNode(list[3].ID)
	if err != nil {
		t.Fatal(err)
	}
	node2, err := GetNode(list[3].Name)
	if err != nil {
		t.Fatal(err)
	}
	if node1 != node2 {
		t.Fatal("Unexpected")
	}

	node1, err = GetNode("node0000")
	if err == nil {
		t.Fatal(err)
	}

	n := Node{ID: list[3].ID}
	err = n.UpdateStatus(9999)
	if err != nil {
		t.Fatal(err)
	}
	node3, err := GetNode(list[3].ID)
	if err != nil {
		t.Fatal(err)
	}

	if n.Status != node3.Status {
		t.Fatalf("Unexpect,%d != %d", n.Status, node3.Status)
	}

	nodes1, err := ListNode(100)
	if err != nil {
		t.Fatal(err)
	}
	for i := range nodes1 {
		if nodes1[i].ID == "" {
			t.Error("nil value")
		}
	}
	if len(nodes1) != 2 {
		t.Fatal("Unexpect")
	}

	nodes2, err := ListNodeByCluster("cluster2")
	if err != nil {
		t.Fatal(err)
	}
	for i := range nodes2 {
		if nodes1[i].ID == "" {
			t.Error("nil value")
		}
	}
	if len(nodes2) != 2 {
		t.Fatal("Unexpect")
	}

	for i := range list {
		IDOrName := list[i].ID
		if i%2 == 0 {
			IDOrName = list[i].Name
		}
		if err := DeleteNode(IDOrName); err != nil {
			t.Fatal(IDOrName, err)
		}
	}

	tasks := []*Task{
		NewTask("qwertyu", "qwertyuiopsdfghjklbn", "tyuighjfdghjkl", nil, 888),
		NewTask("qweafjlafjrtyu", "qwertyuiajlfakfaopsdfghjklbn", "tyuifajflaghjfdghjkl", nil, 888),
		NewTask("qwertyu", "qwertyuiopsdfghjklbn", "tyuighjfdghjkl", []string{"jlafjlakf", "jaljflajflajf"}, 888),
		NewTask("qweafjlahjklkjhfjrtyu", "qwertfajlfjafyuiajlfakfaopsdfghjklbn", "", nil, 888),
	}

	defer func() {
		for i := range tasks {
			if err := DeleteTask(tasks[i].ID); err != nil {
				t.Error(tasks[i].ID, err)
			}
		}
	}()

	err = TxInsertNodeAndTask(&list[0], tasks[0])
	if err != nil {
		t.Fatal(err)
	}

	list1 := make([]*Node, len(list))
	for i := range list {
		list1[i] = &list[i]
	}

	err = TxInsertMultiNodeAndTask(list1[1:], tasks[1:])
	if err != nil {
		t.Fatal(err)
	}

	err = TxUpdateNodeStatus(&list[0], tasks[1], 9977, 999, false, "foafghjk")
	if err != nil {
		t.Fatal(err)
	}

	err = TxUpdateNodeStatus(&list[1], tasks[2], 0, 999, true, "")
	if err != nil {
		t.Fatal(err)
	}

}
