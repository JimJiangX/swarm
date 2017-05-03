package structs

type ResponseHead struct {
	Result  bool        `json:"result"`
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Object  interface{} `json:"object"`
}

type BackupTaskCallback struct {
	TaskID string `json:"task_id"`
	UnitID string `json:"unit_id"`
	Type   string `json:"type,omitempty"`
	Path   string `json:"path,omitempty"`
	Code   byte   `json:"code"`
	Size   int    `json:"size,omitempty"`
	Msg    string `json:"msg,omitempty"`
}
