package models

import (
	"StatsCollector/models"
	"errors"
	"time"

	"github.com/gliderlabs/ssh"
)

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

var ErrAgentNameTooLong = errors.New("Agent name was >1000 characters long")
var ErrAgentPubKeyNotValid = errors.New("The key supplied to create an agent was invalid (not an SSH public key)")

func CreateAgent(Name, PubKey string) error {

	if len(Name) > 1000 {
		return ErrAgentNameTooLong
	}

	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(PubKey))
	if len(PubKey) == 0 || err != nil {
		return ErrAgentPubKeyNotValid
	}

	var newAgent models.Agent
	newAgent.Name = Name
	newAgent.PubKey = PubKey

	return db.Create(&newAgent).Error
}

func DeleteAgent(PubKey string) error {

	var toRemove models.Agent
	if err := db.Find(&toRemove, "pub_key = ?", PubKey).Error; err != nil {
		return err
	}

	db.Where("id = ?", toRemove.Id).Delete(&toRemove)
	db.Delete(&models.MonitorEntry{}, "agent_id = ?", toRemove.Id)
	db.Delete(&models.DiskEntry{}, "agent_id = ?", toRemove.Id)
	db.Delete(&models.Alert{}, "agent_id = ?", toRemove.Id)
	db.Delete(&models.Event{}, "agent_id = ?", toRemove.Id)

	return nil
}

func GetAgent(PubKey string) (Agent, error) {
	var currentAgent Agent

	if err := db.Preload("AlertProfile").
		Preload("Monitors").
		Preload("Disks").
		Preload("SystemInfo").
		Find(&currentAgent, "pub_key = ?", string(PubKey)).Error; err != nil {

		return Agent{}, err
	}

	return currentAgent, nil
}

func GetAgentList(filter string, limit int) (agents []Agent, err error) {

	tx := db
	if len(filter) > 0 {
		tx = tx.Where("currently_connected = ?", filter == "online")
	}

	err = tx.Preload("Monitors").Preload("Disks").Find(&agents).Limit(limit).Error

	return agents, err
}
