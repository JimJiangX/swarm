package database

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type UserOrmer interface {
	InsertUsers(users []User) error

	ListUsersByService(service, _type string) ([]User, error)

	SetUsers(addition, update []User) error

	DelUsers(users []User) error
}

// User is for DB and Proxy
type User struct {
	ReadOnly  bool   `db:"read_only" json:"read_only"`
	ID        string `db:"id"`
	ServiceID string `db:"service_id" json:"service_id"`
	Type      string `db:"type"`
	Username  string `db:"username"`
	Password  string `db:"password"`
	Role      string `db:"role"`

	Blacklist []string `db:"-"`
	Whitelist []string `db:"-"`
	White     string   `db:"whitelist" json:"-"`
	Black     string   `db:"blacklist" json:"-"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (db dbBase) userTable() string {
	return db.prefix + "_users"
}

// ListUsersByService returns []User select by serviceID and User type if assigned
func (db dbBase) ListUsersByService(service, _type string) ([]User, error) {
	var (
		err   error
		users []User
	)

	if _type == "" {

		query := "SELECT id,service_id,type,username,password,role,read_only,blacklist,whitelist,created_at FROM " + db.userTable() + " WHERE service_id=?"
		err = db.Select(&users, query, service)

	} else {

		query := "SELECT id,service_id,type,username,password,role,read_only,blacklist,whitelist,created_at FROM " + db.userTable() + " WHERE service_id=? AND type=?"
		err = db.Select(&users, query, service, _type)

	}
	if err != nil {
		return nil, errors.Wrap(err, "list []User by serviceID")
	}

	for i := range users {
		err = users[i].jsonDecode()
		if err != nil {
			return nil, err
		}
	}

	return users, nil
}

func (u *User) jsonDecode() error {
	u.Blacklist = []string{}
	u.Whitelist = []string{}

	buffer := bytes.NewBufferString(u.Black)
	if len(u.Black) > 0 {
		err := json.NewDecoder(buffer).Decode(&u.Blacklist)
		if err != nil {
			return errors.Wrap(err, "JSON decode blacklist")
		}
	}

	if len(u.White) > 0 {
		buffer.Reset()
		buffer.WriteString(u.White)

		err := json.NewDecoder(buffer).Decode(&u.Whitelist)
		if err != nil {
			return errors.Wrap(err, "JSON decode whitelist")
		}
	}

	return nil
}

func (u *User) jsonEncode() error {
	buffer := bytes.NewBuffer(nil)

	if len(u.Blacklist) > 0 {

		err := json.NewEncoder(buffer).Encode(u.Blacklist)
		if err != nil {
			return errors.Wrap(err, "JSON Encode User.Blacklist")
		}

		u.Black = buffer.String()
	}

	buffer.Reset()

	if len(u.Whitelist) > 0 {

		err := json.NewEncoder(buffer).Encode(u.Whitelist)
		if err != nil {
			return errors.Wrap(err, "JSON Encode User.Whitelist")
		}

		u.White = buffer.String()
	}

	return nil
}

// SetUsers update []User in Tx,
// If User is exist,exec update,if not exec insert.
func (db dbBase) SetUsers(addition, update []User) error {
	do := func(tx *sqlx.Tx) error {

		if len(addition) > 0 {
			err := db.txInsertUsers(tx, addition)
			if err != nil {
				return err
			}
		}

		if len(update) == 0 {
			return nil
		}

		query := "UPDATE " + db.userTable() + " SET type=:type,password=:password,role=:role,read_only=:read_only,blacklist=:blacklist,whitelist=:whitelist WHERE id=:id OR username=:username"
		stmt, err := tx.PrepareNamed(query)
		if err != nil {
			return errors.Wrap(err, "Tx prepare update User")
		}

		for i := range update {
			if err = update[i].jsonEncode(); err != nil {
				stmt.Close()

				return err
			}

			_, err = stmt.Exec(update[i])
			if err != nil {
				stmt.Close()

				return err
			}
		}

		stmt.Close()

		return err
	}

	return db.txFrame(do)
}

// InsertUsers insert []User in Tx
func (db dbBase) InsertUsers(users []User) error {
	return db.txFrame(
		func(tx *sqlx.Tx) (err error) {
			return db.txInsertUsers(tx, users)
		})
}

func (db dbBase) txInsertUsers(tx *sqlx.Tx, users []User) error {

	query := "INSERT INTO " + db.userTable() + " (id,service_id,type,username,password,role,read_only,blacklist,whitelist,created_at) VALUES (:id,:service_id,:type,:username,:password,:role,:read_only,:blacklist,:whitelist,:created_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare insert []User")
	}

	for i := range users {
		if len(users[i].ID) == 0 {
			continue
		}

		if err = users[i].jsonEncode(); err != nil {
			stmt.Close()

			return err
		}

		_, err = stmt.Exec(&users[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx insert []User")
		}
	}

	stmt.Close()

	return errors.Wrap(err, "Tx insert []User")
}

func (db dbBase) txDelUsers(tx *sqlx.Tx, id string) error {

	query := "DELETE FROM " + db.userTable() + " WHERE id=? OR service_id=?"
	_, err := tx.Exec(query, id, id)

	return errors.Wrap(err, "Tx delete User by ID or ServiceID")
}

// DelUsers delete []User in Tx
func (db dbBase) DelUsers(users []User) error {
	do := func(tx *sqlx.Tx) error {

		query := "DELETE FROM " + db.userTable() + " WHERE id=?"
		stmt, err := tx.Preparex(query)
		if err != nil {
			return errors.Wrap(err, "Tx prepare delete []User")
		}

		for i := range users {
			_, err = stmt.Exec(users[i].ID)
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "Tx delete User by ID:"+users[i].ID)
			}
		}

		stmt.Close()

		return err
	}

	return db.txFrame(do)
}
