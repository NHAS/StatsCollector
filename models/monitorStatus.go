package models

//MonitorStatus is the object representing a endpoints status.
//Whether it is up, or down. And if down provides a reason
type MonitorStatus struct {
	Path   string `gorm:"unique;not null"`
	OK     bool
	Reason string

	StatusCode int
}
