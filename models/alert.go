package models

import (
	"StatsCollector/models"
	"errors"
)

//Alert is the alert profile that is associated per agent.
//In the future this will be able to set disk usage warnings as well
type Alert struct {
	Id      int64
	AgentId int64

	Active   bool
	DiskUtil int64
}

var ErrPubKeyEmpty = errors.New("Public Key not set")

func CreateAlertProfileForAgent(agentPubkey string, diskUtilisation int64, active bool) error {
	if len(agentPubkey) == 0 {
		return ErrPubKeyEmpty
	}

	var agent models.Agent
	if err := db.Find(&agent, "pub_key = ?", string(agentPubkey)).Error; err != nil {
		return err
	}

	newAlert := models.Alert{
		AgentId:  agent.Id,
		DiskUtil: diskUtilisation,
		Active:   active,
	}

	var alertID []int64
	if err := db.Find(&models.Alert{}, "agent_id = ?", agent.Id).Pluck("id", &alertID).Error; err == nil {
		newAlert.Id = alertID[0]
	}

	return db.Debug().Save(&newAlert).Error
}
