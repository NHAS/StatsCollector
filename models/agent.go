package models

import (
	"StatsCollector/models"
	"errors"
	"time"

	"github.com/gliderlabs/ssh"
)

//Agent is the overarching database structure tying all client metrics together
type Agent struct {
	ID                 int64
	Name               string
	PubKey             string `gorm:"unique;not null"`
	LastTransmission   time.Time
	LastConnectionFrom string
	CurrentlyConnected bool

	SystemInfo   SystemInfo
	AlertProfile Alert
	Events       []Event

	MemoryUsage float32
	Disks       []DiskEntry    `gorm:"PRELOAD:true"`
	Monitors    []MonitorEntry `gorm:"PRELOAD:true"`
}

//ErrAgentNameTooLong is returned when an agent name is too long
var ErrAgentNameTooLong = errors.New("Agent name was >1000 characters long")

//ErrAgentPubKeyNotValid is returned when an ssh key given for an agent is not parsable as a public key
var ErrAgentPubKeyNotValid = errors.New("The key supplied to create an agent was invalid (not an SSH public key)")

//CreateAgent parses a public key and adds a new agent to the database.
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

//DeleteAgent removes an agent and any of its relationed structures (such as disk information) from the database
//Todo, this is currently a fragile way of doing this, as we have to keep adding more "delete" statements. GORM probably has a way of doing this. But am unsure
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
	db.Delete(&models.SystemInfo{}, "agent_id = ?", toRemove.Id)

	return nil
}

//GetAgent returns an agent from the database with the matching Public Key
//It also loads all the associated structures, such as monitors and disk information
func GetAgent(PubKey string) (Agent, error) {
	var currentAgent Agent

	if err := db.Preload("AlertProfile").
		Preload("Monitors").
		Preload("Disks").
		Preload("SystemInfo").
		Preload("Events").
		Find(&currentAgent, "pub_key = ?", string(PubKey)).Error; err != nil {

		return Agent{}, err
	}

	return currentAgent, nil
}

//GetAgentList returns a limited number of agents with a filter whether they are connected or not.
func GetAgentList(filter string, limit int) (agents []Agent, err error) {

	tx := db
	if len(filter) > 0 {
		tx = tx.Where("currently_connected = ?", filter == "online")
	}

	err = tx.Preload("Monitors").Preload("Disks").Order("id asc").Find(&agents).Limit(limit).Error

	return agents, err
}

//GetDashboardInformation gets the agents that are up or down and those that have failed endpoints.
// This is used in the dashboard
func GetDashboardInformation() (totalAgents int, downAgents []Agent, degradedAgents []Agent, failedEndPoints []MonitorEntry, err error) {

	err = db.Model(&models.Agent{}).Count(&totalAgents).Error
	if err != nil {
		goto failed
	}

	err = db.Find(&downAgents, "currently_connected = ?", false).Error
	if err != nil {
		goto failed
	}

	err = db.Select("DISTINCT agents.*").
		Joins("INNER JOIN monitor_entries ON agents.id = monitor_entries.agent_id").
		Find(&degradedAgents, "(NOT monitor_entries.ok) AND agents.currently_connected").Error
	if err != nil {
		goto failed
	}

	err = db.Find(&failedEndPoints, "ok = ?", false).Error
	if err != nil {
		goto failed
	}

	return totalAgents, downAgents, degradedAgents, failedEndPoints, nil

failed:
	return 0, []Agent{}, []Agent{}, []MonitorEntry{}, err
}
