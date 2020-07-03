package models

//DiskEntry is the used percentage of the disk for the database
type DiskEntry struct {
	Id      int64
	AgentId int64

	Device string `gorm:"unique;not null"`
	Usage  float32
}
