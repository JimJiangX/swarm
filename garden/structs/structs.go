package structs

type ResponseHead struct {
	Result   bool        `json:"result"`
	Code     int         `json:"code"`
	Error    string      `json:"msg"`
	Category string      `json:"category"`
	Object   interface{} `json:"object"`
}

type BackupTaskCallback struct {
	TaskID    string `json:"task_id"`
	UnitID    string `json:"unit_id"`
	Type      string `json:"type,omitempty"`
	Path      string `json:"path,omitempty"`
	Remark    string `json:"remark,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Msg       string `json:"msg,omitempty"`
	Retention int    `json:"retention"`
	Size      int    `json:"size,omitempty"`
	Code      int    `json:"code,omitempty"`
}
