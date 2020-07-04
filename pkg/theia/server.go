package theia

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"time"

	"github.com/NHAS/StatsCollector/models"
	"github.com/NHAS/StatsCollector/utils"
	"github.com/NHAS/StatsCollector/webservice"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres" // Imports the postgres dialect for gorm to use

	"golang.org/x/crypto/ssh"
)

type ServerConfig struct {
	CollectionListenAddr string `json:"ssh_listen_addr"`
	WebListenAddr        string `json:"web_interface_addr"`
	PrivateKeyPath       string `json:"private_key_path"`
}

func RunServer(db *gorm.DB, config ServerConfig) {

	
	models.InitaliseModels(db)

	db.Model(&models.Agent{}).Update("currently_connected", false)

	// An SSH server is represented by a ServerConfig, which holds
	// certificate details and handles authentication of ServerConns.
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			authorizedKeysMap := map[string]string{}

			var authorisedKeys []string
			if err := db.Find(&models.Agent{}).Pluck("pub_key", &authorisedKeys).Error; err != nil && err != gorm.ErrRecordNotFound {
				utils.Check("Unable to load public keys from database", err)
			}

			for _, v := range authorisedKeys {
				parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(v))
				utils.Check("Unable to parse public authorised key", err)

				authorizedKeysMap[string(parsedKey.Marshal())] = v
			}

			if key, ok := authorizedKeysMap[string(pubKey.Marshal())]; ok {
				return &ssh.Permissions{
					// Record the public key used for authentication.
					Extensions: map[string]string{
						"pubkey-fp": key,
					},
				}, nil
			}
			return nil, fmt.Errorf("Unknown public key for %q", c.User())
		},
	}

	privateBytes, err := ioutil.ReadFile(config.PrivateKeyPath)
	utils.Check("Failed to load private key: ", err)

	private, err := ssh.ParsePrivateKey(privateBytes)
	utils.Check("Failed to parse private key: ", err)

	serverConfig.AddHostKey(private)

	// Once a ServerConfig has been configured, connections can be
	// accepted.

	listener, err := net.Listen("tcp", config.CollectionListenAddr)
	utils.Check("Failed to listen for connection: ", err)

	log.Println("Starting web interface")
	webservice.StartWebServer(config.WebListenAddr, db)

	//log.Println("Starting emailer")
	//go startEventProcessors(db)

	log.Println("Now accepting connections on ", listener.Addr().String())
	for {

		nConn, err := listener.Accept()
		if err != nil {
			log.Println("Failed to accept incoming connection: ", err)
		}

		go handleAgentConnection(nConn, serverConfig, db)

	}
}

func handleAgentConnection(nConn net.Conn, config *ssh.ServerConfig, db *gorm.DB) {
	// Before use, a handshake must be performed on the incoming
	// net.Conn.
	conn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		log.Println("Failed to handshake: ", err)
		return
	}
	defer conn.Close()
	publicKey := conn.Permissions.Extensions["pubkey-fp"]

	log.Printf("Client connected [%s]", publicKey)

	var clientAgent models.Agent
	if err := db.Where("pub_key = ?", publicKey).First(&clientAgent).Error; err != nil {

		log.Println("Something went wrong finding the agent associated with the pub key: ", err)
		return
	}

	// The incoming Request channel must be serviced.
	go ssh.DiscardRequests(reqs)

	// Service the incoming Channel channel.
	for newChannel := range chans {
		// Channels have a type, depending on the application level
		// protocol intended. In the case of a shell, the type is
		// "session" and ServerShell may be used to present a simple
		// terminal interface.
		if newChannel.ChannelType() != "metrics" {
			newChannel.Reject(ssh.Prohibited, "Unable to open channel")
			continue
		}

		log.Println("Handling new channel: ", string(newChannel.ExtraData()), " ", newChannel.ChannelType())

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Println("Could not accept channel: ", err)
			continue
		}

		if err := db.Model(&clientAgent).Updates(models.Agent{LastConnectionFrom: nConn.RemoteAddr().String()}).Error; err != nil {
			log.Println("Updating last connected ip failed: ", err)
			return
		}

		go func() {
			defer channel.Close()
			defer func(db *gorm.DB) {
				timestamp := time.Now()
				message := "Agent: " + clientAgent.Name + "\n\t"
				message += clientAgent.PubKey + "\n\t"
				message += clientAgent.LastConnectionFrom + "\n"
				message += "Is offline (" + timestamp.Format("15:04:05 Jan 2 Mon") + ")\n"

				db.Create(&models.Event{
					AgentId:   clientAgent.Id,
					Message:   message,
					CreatedAt: timestamp,
				})
			}(db)

			decoder := json.NewDecoder(channel)

			for {
				var stat models.Stats
				err := decoder.Decode(&stat)
				if err != nil {
					log.Printf("Client [%s] sent something I couldnt decode, killing", publicKey)
					if err := db.Model(&clientAgent).Update("currently_connected", false).Error; err != nil {
						log.Println("Unsetting currently connected failed: ", err)
						continue
					}

					return
				}

				update := models.Agent{
					LastTransmission:   time.Now(),
					CurrentlyConnected: true,
					MemoryUsage:        stat.MemoryUsage,
				}

				if err := db.Model(&clientAgent).Updates(update).Error; err != nil {
					log.Println("Updating database failed: ", err)
					continue
				}

				for device, usage := range stat.DiskUsage {
					var entry models.DiskEntry
					if err := db.Where("device = ? AND agent_id = ?", device, clientAgent.Id).First(&entry).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							if err := db.Create(&models.DiskEntry{
								AgentId: clientAgent.Id,
								Device:  device,
								Usage:   usage,
							}).Error; err != nil {
								log.Println("Unable to create new disk device: ", err)

							}
							continue
						}

						log.Println("Error adding disk to database:", err)
						continue
					}

					if entry.Usage != usage {
						if err := db.Model(&entry).Update("usage", usage).Error; err != nil {
							log.Println("Error doing the update for disk:", err)
						}
					}
				}

				for _, monitorV := range stat.MonitorValues {
					var me models.MonitorEntry

					if err := db.Where("path = ? AND agent_id = ?", monitorV.Path, clientAgent.Id).First(&me).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							if err := db.Create(&models.MonitorEntry{
								AgentId:      clientAgent.Id,
								MonitorEntry: monitorV,
							}).Error; err != nil {
								log.Println("Unable to create new monitor entry: ", err)

							}
							continue
						}

						log.Println("An error occur update the monitor stat: ", err)
						continue

					}

					if err := db.Model(&me).Updates(&models.MonitorEntry{
						MonitorEntry: monitorV,
					}).Error; err != nil {
						log.Println("Error: ", err)
					}
				}

			}
		}()

		// Sessions have out-of-band requests such as "shell",
		// "pty-req" and "env".  Here we handle only the
		// "shell" request.
		go func(in <-chan *ssh.Request) {
			for req := range in {
				switch req.Type {
				case "system":
					if err := storeSystemAttributes(clientAgent.Id, req.Payload, db); err != nil {
						log.Printf("Client [%s] sent something I couldnt decode, killing", publicKey)
						channel.Close()
						return
					}
				default:
					log.Println("Client sent something... but what...: ", req.Type)
				}
			}
		}(requests)
	}
}

func storeSystemAttributes(agentID int64, b []byte, db *gorm.DB) error {
	var sysinfo models.SystemInfo

	if err := db.Find(&sysinfo, "agent_id = ?", agentID).Error; err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	sysinfo.AgentId = agentID

	err := json.Unmarshal(b, &sysinfo)
	if err != nil {
		return err
	}

	return db.Save(&sysinfo).Error
}

func startEventProcessors(db *gorm.DB) {
	var notification models.NotificationDetail
	if err := db.First(&notification).Error; err != nil {
		log.Println("Unable to find details of how to notify:", err)
		return
	}

	host, _, _ := net.SplitHostPort(notification.EmailProviderHost)

	auth := smtp.PlainAuth("", notification.SendAddress, notification.AccountPassword, host)

	from := mail.Address{"", notification.SendAddress}
	to := mail.Address{"", notification.Destination}

	for {

		var events []models.Event
		if err := db.Find(&events, "notified = false AND urgency < 2").Error; err == nil {

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

				subj := "A host encountered an issue (Urgency: " + fmt.Sprintf("%d", e.Urgency) + ")"

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
		} else {
			log.Println(err)
		}

		<-time.After(30 * time.Second)
	}
}
