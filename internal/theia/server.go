package theia

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"time"

	"github.com/NHAS/StatsCollector/internal/theia/webservice"
	"github.com/NHAS/StatsCollector/models"
	"github.com/NHAS/StatsCollector/utils"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres" // Imports the postgres dialect for gorm to use

	"golang.org/x/crypto/ssh"
)

// ServerConfig is the structure containing the various listening addresses and the private key
type ServerConfig struct {
	CollectionListenAddr string `json:"ssh_listen_addr"`
	WebListenAddr        string `json:"web_interface_addr"`
	PrivateKeyPath       string `json:"private_key_path"`
	WebResourcesPath     string `json:"web_path"`
}

//RunServer starts the webserver, ssh server (collector) and the event database notifier
func RunServer(db *gorm.DB, config ServerConfig) {

	log.Println("Starting in server mode")

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
	webservice.StartWebServer(config.WebListenAddr, config.WebResourcesPath, db)

	log.Println("Starting event processor")
	go startEventProcessors(db)

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
					if err := db.Where("device = ? AND agent_id = ?", device, clientAgent.ID).First(&entry).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							if err := db.Create(&models.DiskEntry{
								AgentId: clientAgent.ID,
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

					if err := db.Where("path = ? AND agent_id = ?", monitorV.Path, clientAgent.ID).First(&me).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							if err := db.Create(&models.MonitorEntry{
								AgentId:      clientAgent.ID,
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
					if err := storeSystemAttributes(clientAgent.ID, req.Payload, db); err != nil {
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
