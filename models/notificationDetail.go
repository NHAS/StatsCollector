package models

import "time"

//NotificationDetail is a structure to store user notification preferences in the database
type NotificationDetail struct {
	Id        int64
	UserId    int64
	UpdatedAt time.Time

	Destination       string
	SendAddress       string
	AccountPassword   string
	EmailProviderHost string
}
