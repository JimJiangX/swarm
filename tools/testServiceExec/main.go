package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Exec struct {
	Name string   `json:"nameOrID"`
	Cmd  []string `json:"cmd"`
}

func main() {
	exec := Exec{
		Name: "624a245d_abcde001",
		Cmd: []string{
			"/root/effect-config.sh", "upredis", `save="100 1000"`,
		},
	}

	buf := bytes.NewBuffer(nil)
	err := json.NewEncoder(buf).Encode(exec)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := http.Post(
		"http://146.4.71.11:20152/v1.0/services/943525cf5bd6fcfe2122774759d53ef4/exec",
		"application/json",
		buf,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var val = struct {
		Task string `json:"task_id"`
	}{}

	err = json.NewDecoder(resp.Body).Decode(&val)
	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		time.Sleep(10 * time.Second)

		task, err := getTask(val.Task)
		if err != nil {
			log.Println(err)
			continue
		}

		log.Println(task.Status, task.FinishedAt, task.Errors)

		if task.Status > 4 {
			break
		}
	}
}

type Task struct {
	ID         string `db:"id" json:"id"`
	Name       string `db:"name" json:"name"` //Related-Object
	Related    string `db:"related" json:"related"`
	Linkto     string `db:"link_to" json:"link_to"`
	LinkTable  string `db:"link_table" json:"-"`
	Desc       string `db:"description" json:"description"`
	Labels     string `db:"labels" json:"labels"`
	Errors     string `db:"errors" json:"errors"`
	Status     int    `db:"status" json:"status"`
	Timestamp  int64  `db:"timestamp" json:"timestamp"` // time.Time.Unix()
	Timeout    int64  `db:"timeout" json:"timeout"`
	CreatedAt  string `db:"created_at" json:"created_at"`
	FinishedAt string `db:"finished_at" json:"finished_at"`
}

func getTask(task string) (Task, error) {
	resp, err := http.Get("http://146.4.71.11:20152/v1.0/tasks/" + task)
	if err != nil {
		return Task{}, err
	}
	defer resp.Body.Close()

	t := Task{}

	err = json.NewDecoder(resp.Body).Decode(&t)

	return t, err
}
