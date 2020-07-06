package models

import (
	"github.com/jinzhu/gorm"
)

var db *gorm.DB

//InitaliseModels initalisaties the database with all models, this creates the tables if they do not exist.
//It also sets the package global 'db' variable, otherwise a pointer deference error will occur
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
