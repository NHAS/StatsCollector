package models

import (
	"StatsCollector/models"

	"github.com/jinzhu/gorm"
)

func InitaliseModels(db *gorm.DB) {

	db.AutoMigrate(&models.Event{},
		&models.Agent{},
		&models.MonitorEntry{},
		&models.DiskEntry{},
		&models.NotificationDetail{},
		&models.SystemInfo{},
		&models.Alert{})
}
