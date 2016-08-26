package database

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertImageQuery = "INSERT INTO tb_image (enabled,id,name,version,docker_image_id,label,size,template_config_id,config_keysets,upload_at) VALUES (:enabled,:id,:name,:version,:docker_image_id,:label,:size,:template_config_id,:config_keysets,:upload_at)"

// Image table tb_image structure
type Image struct {
	Enabled bool   `db:"enabled"`
	ID      string `db:"id"`
	Name    string `db:"name"`
	Version string `db:"version"`
	ImageID string `db:"docker_image_id"`
	Labels  string `db:"label"`
	Size    int    `db:"size"`

	TemplateConfigID string `db:"template_config_id"`
	ConfigKeySets    string `db:"config_keysets"` // map[string]KeysetParams

	UploadAt time.Time `db:"upload_at"`
}

func (Image) tableName() string {
	return "tb_image"
}

// TxInsertImage insert Image and UnitConfig in Tx
func TxInsertImage(image Image, config UnitConfig) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	image.ConfigKeySets, err = unitConfigEncode(config.KeySets)

	_, err = tx.NamedExec(insertImageQuery, &image)
	if err != nil {
		return errors.Wrap(err, "TX insert Image")
	}

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return errors.Wrap(err, "Tx insert UnitConfig")
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx insert image & UnitConfig")
}

// ListImages returns Image slice select for DB.
func ListImages() ([]Image, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var images []Image
	const query = "SELECT * FROM tb_image"

	err = db.Select(&images, query)
	if err == nil {
		return images, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&images, query)

	return images, errors.Wrap(err, "list []Image")
}

// GetImage returns Image select by name and version.
func GetImage(name, version string) (Image, error) {
	db, err := GetDB(false)
	if err != nil {
		return Image{}, err
	}

	image := Image{}
	const query = "SELECT * FROM tb_image WHERE name=? AND version=?"

	err = db.Get(&image, query, name, version)
	if err == nil {
		return image, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return Image{}, err
	}

	err = db.Get(&image, query, name, version)

	return image, errors.Wrap(err, "get Image")
}

// txGetImage returns Image select by ID or ImageID in Tx.
func txGetImage(tx *sqlx.Tx, ID string) (image Image, err error) {
	err = tx.Get(&image, "SELECT * FROM tb_image WHERE id=? OR docker_image_id=?", ID, ID)

	return image, errors.Wrap(err, "Tx get Image by ID")
}

// GetImageAndUnitConfig returns Image and UnitConfig,
// select by ID or ImageID.
func GetImageAndUnitConfig(ID string) (Image, UnitConfig, error) {
	image := Image{}
	config := UnitConfig{}

	db, err := GetDB(true)
	if err != nil {
		return image, config, err
	}

	err = db.Get(&image, "SELECT * FROM tb_image WHERE id=? OR docker_image_id=?", ID, ID)
	if err != nil {
		return image, config, errors.Wrap(err, "get Image by ID")
	}

	err = db.Get(&config, "SELECT * FROM tb_unit_config WHERE id=?", image.TemplateConfigID)
	if err != nil {
		return image, config, errors.Wrap(err, "get UnitConfig by ID")
	}

	config.KeySets, err = unitConfigDecode(image.ConfigKeySets)

	return image, config, err
}

// GetImageByID returns Image select by ID or ImageID
func GetImageByID(ID string) (Image, error) {
	db, err := GetDB(false)
	if err != nil {
		return Image{}, err
	}

	image := Image{}
	const query = "SELECT * FROM tb_image WHERE id=? OR docker_image_id=?"

	err = db.Get(&image, query, ID, ID)
	if err == nil {
		return image, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return Image{}, err
	}

	err = db.Get(&image, query, ID, ID)

	return image, errors.Wrap(err, "get Image by ID")
}

// UpdateImageStatus update Image.Enabled by ID or ImageID.
func UpdateImageStatus(ID string, enable bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_image SET enabled=? WHERE id=? OR docker_image_id=?"
	_, err = db.Exec(query, enable, ID, ID)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, enable, ID, ID)

	return errors.Wrap(err, "update Image by ID")
}

func isImageUsed(tx *sqlx.Tx, image string) (bool, error) {
	var out []string
	const query = "SELECT unit_id FROM tb_unit_config WHERE image_id=?"

	err := tx.Select(&out, query, image)
	if err != nil {
		return false, errors.Wrap(err, "select []UnitConfig")
	}

	exist := false
	for i := range out {
		if strings.TrimSpace(out[i]) != "" {
			exist = true
			break
		}
	}

	return exist, nil
}

// TxDeleteImage delete Image and UnitConfig by ID in Tx.
func TxDeleteImage(ID string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	image, err := txGetImage(tx, ID)
	if err != nil {
		if ErrNoRowsFound == CheckError(err) {
			return nil
		}

		return err
	}

	ok, err := isImageUsed(tx, image.ID)
	if err != nil {
		return err
	}
	if ok {
		return errors.Errorf("Image %s is using", ID)
	}

	_, err = tx.Exec("DELETE FROM tb_image WHERE id=? OR docker_image_id=?", ID, ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM tb_unit_config WHERE image_id=?", image.ID)
	if err != nil {
		return errors.Wrapf(err, "Tx delete UnitConfig by ImageID:%s", image.ID)
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx delete Image by ID")
}

const insertUnitConfigQuery = "INSERT INTO tb_unit_config (id,image_id,unit_id,config_file_path,version,parent_id,content,created_at) VALUES (:id,:image_id,:unit_id,:config_file_path,:version,:parent_id,:content,:created_at)"

// UnitConfig is assciated to Image and Unit
type UnitConfig struct {
	ID        string                  `db:"id"`
	ImageID   string                  `db:"image_id"`
	UnitID    string                  `db:"unit_id"`
	Mount     string                  `db:"config_file_path"`
	Version   int                     `db:"version"`
	ParentID  string                  `db:"parent_id"`
	Content   string                  `db:"content"`
	KeySets   map[string]KeysetParams `db:"-"`
	CreatedAt time.Time               `db:"created_at"`
}

// KeysetParams is UnitConfig Content option
type KeysetParams struct {
	Key         string
	CanSet      bool
	MustRestart bool
	Description string
}

func (u UnitConfig) tableName() string {
	return "tb_unit_config"
}

func unitConfigEncode(keysets map[string]KeysetParams) (string, error) {
	if len(keysets) == 0 {
		return "", nil
	}
	buffer := bytes.NewBuffer(nil)
	err := json.NewEncoder(buffer).Encode(keysets)

	return buffer.String(), errors.Wrap(err, "UnitConfig encode")
}

func unitConfigDecode(src string) (map[string]KeysetParams, error) {
	if len(src) == 0 {
		return map[string]KeysetParams{}, nil
	}
	var val map[string]KeysetParams
	dec := json.NewDecoder(strings.NewReader(src))
	err := dec.Decode(&val)

	return val, errors.Wrap(err, "UnitConfig decode")
}

// GetUnitConfigByID returns *UnitConfig select by ID
func GetUnitConfigByID(ID string) (*UnitConfig, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	config := &UnitConfig{}
	err = db.Get(config, "SELECT * FROM tb_unit_config WHERE id=?", ID)
	if err != nil {
		return nil, errors.Wrap(err, "get UnitConfig")
	}

	var keysets string
	err = db.Get(&keysets, "SELECT config_keysets FROM tb_image WHERE id=?", config.ImageID)
	if err != nil {
		return nil, errors.Wrapf(err, "Get Image.ConfigKeysets by id='%s'", config.ImageID)
	}

	config.KeySets, err = unitConfigDecode(keysets)

	return config, err
}

// UnitWithConfig contains Unit and UnitConfig
type UnitWithConfig struct {
	Unit   Unit
	Config UnitConfig
}

// ListUnitConfigByService returns []UnitWithConfig belongs to service.
func ListUnitConfigByService(service string) ([]UnitWithConfig, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var units []Unit
	err = db.Select(&units, "SELECT * FROM tb_unit WHERE service_id=?", service)
	if err != nil {
		return nil, errors.Wrapf(err, "Select []Unit by service_id='%s'", service)
	}

	if len(units) == 0 {
		return []UnitWithConfig{}, nil
	}

	ids := make([]string, len(units))
	for i := range units {
		ids[i] = units[i].ConfigID
	}

	query, args, err := sqlx.In("SELECT * FROM tb_unit_config WHERE id IN (?);", ids)
	if err != nil {
		return nil, errors.Wrap(err, "select UnitConfig by ID IN Service Units ID")
	}

	var configs []*UnitConfig
	err = db.Select(&configs, query, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "Select []UnitConfig by id=%s", ids)
	}

	if len(configs) == 0 {
		return nil, nil
	}

	var result []struct {
		ID      string `db:"id"`
		KeySets string `db:"config_keysets"`
	}
	err = db.Select(&result, "SELECT id,config_keysets FROM tb_image")
	if err != nil {
		return nil, errors.Wrap(err, "Select []Image")
	}

	for i := range configs {
		for r := range result {
			if configs[i].ImageID != result[r].ID {
				continue
			}
			configs[i].KeySets, err = unitConfigDecode(result[r].KeySets)
			if err == nil {
				break
			}
		}
	}

	out := make([]UnitWithConfig, 0, len(units))

	for i := range units {
		for _, c := range configs {
			if c == nil {
				break
			}

			if units[i].ConfigID != c.ID {
				continue
			}
			out = append(out, UnitWithConfig{
				Unit:   units[i],
				Config: *c,
			})
			break
		}
	}

	return out, nil
}

// TXInsertUnitConfig insert UnitConfig in Tx
func TXInsertUnitConfig(tx *sqlx.Tx, config *UnitConfig) error {
	_, err := tx.NamedExec(insertUnitConfigQuery, config)

	return errors.Wrap(err, "Tx insert UnitConfig")
}

// TxUpdateImageTemplateConfig update Image.TemplateConfigID and insert UnitConfig in Tx.
func TxUpdateImageTemplateConfig(image string, config UnitConfig) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return err
	}

	if keysets := ""; len(config.KeySets) == 0 {
		_, err = tx.Exec("UPDATE tb_image SET template_config_id=? WHERE id=?", config.ID, image)
	} else {
		keysets, err = unitConfigEncode(config.KeySets)
		if err != nil {
			return err
		}

		_, err = tx.Exec("UPDATE tb_image SET template_config_id=?,config_keysets=? WHERE id=?", config.ID, keysets, image)
	}

	if err != nil {
		return errors.Wrap(err, "Tx update Image")
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx update Image Template Config")
}

func txDeleteUnitConfigByUnit(tx *sqlx.Tx, unitID string) error {
	_, err := tx.Exec("DELETE FROM tb_unit_config WHERE unit_id=?", unitID)

	return errors.Wrap(err, "Tx delete UnitConfig by unit ID")
}
