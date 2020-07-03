package models

//MonitorEntry is a wrapper for an object passed from client -> server.
type MonitorEntry struct {
	Id      int64
	AgentId int64

	MonitorEntry MonitorStatus `gorm:"embedded"`
}
