package models

import "time"

//Event is a log/event that has occured from one of the clients
//This is tied into email notifications
type Event struct {
	Id        int64
	AgentId   int64
	UserId    int64
	Urgency   int
	Message   string
	Notified  bool
	CreatedAt time.Time
}
