package models

import (
	"github.com/jinzhu/gorm"
)

var db *gorm.DB

func InitaliseModels(Inputdb *gorm.DB) {

	db = Inputdb

	db.AutoMigrate(
		&Event{},
		&Agent{},
		&MonitorEntry{},
		&DiskEntry{},
		&NotificationDetail{},
		&SystemInfo{},
		&Alert{},
		&User{},
	)
}
