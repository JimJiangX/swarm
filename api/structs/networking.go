package structs

type PostNetworkingRequest struct {
	Prefix  int
	Num     int
	IP      string
	Type    string
	Gateway string
}

type PostImportPortRequest struct {
	Start   int
	End     int
	Filters []int
}
