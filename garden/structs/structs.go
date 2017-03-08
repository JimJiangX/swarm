package structs

type ResponseHead struct {
	Result  bool        `json:"result"`
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Object  interface{} `json:"object"`
}
