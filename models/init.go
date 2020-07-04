package models

import (
	"github.com/jinzhu/gorm"
)

func InitaliseModels(db *gorm.DB) {

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
