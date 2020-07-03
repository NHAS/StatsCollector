package models

//Alert is the alert profile that is associated per agent.
//In the future this will be able to set disk usage warnings as well
type Alert struct {
	Id      int64
	AgentId int64

	Active   bool
	DiskUtil int64
}
