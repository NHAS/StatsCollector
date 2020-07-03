package models

import "time"

//Agent is the overarching database structure tying all client metrics together
type Agent struct {
	Id                 int64
	Name               string
	PubKey             string `gorm:"unique;not null"`
	LastTransmission   time.Time
	LastConnectionFrom string
	CurrentlyConnected bool

	SystemInfo   SystemInfo
	AlertProfile Alert

	MemoryUsage float32
	Disks       []DiskEntry    `gorm:"PRELOAD:true"`
	Monitors    []MonitorEntry `gorm:"PRELOAD:true"`
}
