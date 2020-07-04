package theia

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"time"

	"github.com/NHAS/StatsCollector/models"
	"github.com/jinzhu/gorm"
)

func sendEvent(db *gorm.DB, agentID int64, urgency int, title, message string) error {
	t := time.Now()

	cooldown := t.Add(-2 * time.Hour)

	var num int64
	if err := db.Model(&models.Event{}).Where("created_at > ? AND title = ? AND agent_id = ?", cooldown, title, agentID).Count(&num).Error; err != nil {
		return err
	}

	if num > 0 {
		log.Println("Ratelimiting message as it has occured within 2 hours")
		return nil
	}

	return db.Create(&models.Event{AgentId: agentID, Urgency: urgency, Title: title, Message: message}).Error
}

func eventGenerator(db *gorm.DB) {
	time.Sleep(1 * time.Minute) // Wait for things to connect before just saying theyre dead
	for {
		timeout := time.Now().Add(-10 * time.Minute)

		var agentsWithIssues []models.Agent
		if err := db.Debug().Preload("Monitors").Preload("Disks").Preload("AlertProfile").
			Select("DISTINCT agents.*").
			Joins("INNER JOIN monitor_entries ON agents.id = monitor_entries.agent_id").
			Joins("INNER JOIN disk_entries ON agents.id = disk_entries.agent_id").
			Joins("INNER JOIN alerts ON agents.id = alerts.agent_id").
			Find(&agentsWithIssues,
				"alerts.active AND (agents.last_transmission < ? OR disk_entries.usage > alerts.disk_util OR NOT monitor_entries.ok)", timeout).
			Error; err != nil {
			log.Println("Error loading database things: ", err)
			return
		}

		for _, a := range agentsWithIssues {
			title := ""

			message := "Agent: " + a.PubKey + "\n"
			if len(a.Name) > 0 {
				message += "Friendly Name: " + a.Name + "\n"
				title += a.Name + " "
			} else {
				title += "Agent "
			}
			if len(a.LastConnectionFrom) > 0 {
				message += "Last Connection: " + a.LastConnectionFrom + "\n"
			}

			message += "Last Transmission: " + a.LastTransmission.Format("Mon Jan 2 15:04") + "\n"
			if !a.CurrentlyConnected {
				title += " is offline"
			}

			message += "\nEndpoint Status\n"

			endpointsDown := 0
			for _, m := range a.Monitors {

				message += "\t" + m.MonitorEntry.Path + "\n\tStatus: "
				if !m.MonitorEntry.OK {
					message += "Down. Reason: " + m.MonitorEntry.Reason + "\n"
					endpointsDown++
					continue
				}

				message += "Up.\n"

			}

			message += "\nDisks\n"

			disksAboveUsage := 0
			for _, d := range a.Disks {
				if int64(d.Usage) > a.AlertProfile.DiskUtil {
					disksAboveUsage++
				}
				message += "\t" + d.Device + " Usage: " + fmt.Sprintf("%.02f", d.Usage) + "\n"
			}

			if a.CurrentlyConnected {
				title += "has " + fmt.Sprintf("%d endpoints down, %d disks over usage", endpointsDown, disksAboveUsage)
			}

			if err := sendEvent(db, a.Id, 1, title, message); err != nil {
				log.Println("Unable to send event, dying: ", err)
				return
			}

		}
		time.Sleep(5 * time.Minute)
	}
}

func startEventProcessors(db *gorm.DB) {
	var notification models.NotificationDetail
	for {

		err := db.First(&notification).Error
		if err == nil {
			break
		}
		log.Println("Unable to find details of how to notify:", err)
		time.Sleep(30 * time.Second)
	}

	host, _, _ := net.SplitHostPort(notification.EmailProviderHost)

	auth := smtp.PlainAuth("", notification.SendAddress, notification.AccountPassword, host)

	from := mail.Address{Name: "", Address: notification.SendAddress}
	to := mail.Address{Name: "", Address: notification.Destination}

	go eventGenerator(db)

	for {

		var events []models.Event
		if err := db.Find(&events, "notified = false AND urgency < 2").Error; err == nil && len(events) > 0 {

			// Here is the key, you need to call tls.Dial instead of smtp.Dial
			// for smtp servers running on 465 that require an ssl connection
			// from the very beginning (no starttls)
			c, err := smtp.Dial(notification.EmailProviderHost)
			if err != nil {
				log.Fatalln(err)
			}

			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				log.Fatalln("Start tls failed:", err)
			}

			// Auth
			if err = c.Auth(auth); err != nil {
				log.Fatalln(err)
			}

			for _, e := range events {

				// To && From
				if err = c.Mail(from.Address); err != nil {
					log.Fatalln(err)
				}

				if err = c.Rcpt(to.Address); err != nil {
					log.Fatalln(err)
				}

				// Data
				w, err := c.Data()
				if err != nil {
					log.Fatalln(err)
				}

				subj := e.Title + " (Urgency: " + fmt.Sprintf("%d", e.Urgency) + ")"

				// Setup headers
				headers := make(map[string]string)
				headers["From"] = from.String()
				headers["To"] = to.String()
				headers["Subject"] = subj

				// Setup message
				message := ""
				for k, v := range headers {
					message += fmt.Sprintf("%s: %s\r\n", k, v)
				}
				message += "\r\n" + e.Message

				_, err = w.Write([]byte(message))
				if err != nil {
					log.Fatalln(err)
				}

				err = w.Close()
				if err != nil {
					log.Fatalln(err)
				}

				if err := db.Model(&e).Update("notified", true).Error; err != nil {
					log.Fatal("Going to send too many emails if this fails. So die: ", err)
				}

				<-time.After(1 * time.Second)
				log.Println("Email sent")
			}

			c.Quit()
			log.Println("Disconnecting")
		}

		<-time.After(5 * time.Minute)
	}
}
