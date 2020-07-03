package models

// SystemInfo is a structure passed through the ssh requests on startup of the client.
//  It provides more static system information to the server.
type SystemInfo struct {
	Id            int64 `json:"-"`
	AgentId       int64 `json:"-"`
	CpuCores      int
	TotalMemory   uint64
	KernelVersion string

	Platform string
	Family   string
	Version  string
}
