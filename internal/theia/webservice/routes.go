package webservice

import (
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/NHAS/StatsCollector/models"
	"github.com/NHAS/StatsCollector/utils"
	"github.com/gin-gonic/gin"
	"github.com/gliderlabs/ssh"
	"github.com/gorilla/csrf"
	"github.com/jinzhu/gorm"
	"golang.org/x/crypto/bcrypt"
)

const (
	TokenSize  = 64
	CookieName = "auth"
)

func StartWebServer(listenAddr, templates string, db *gorm.DB) {

	r := gin.Default()
	r.SetFuncMap(template.FuncMap{
		"humanDate":  humanDate,
		"humanTime":  humanTime,
		"limitPrint": limitPrint,
		"Wrap":       wrap,
		"Hex":        hexEncode,
	})

	r.GET("/", index(db))
	setupSessionRoutes(r, db)

	db.AutoMigrate(&models.User{})

	r.LoadHTMLGlob(templates + "/*/*.templ.html")

	CSRF := csrf.Protect([]byte("189734oiylkasJHKUY"), csrf.Secure(false))

	r.Use(authorisionMiddleware(db))

	r.GET("/dashboard", getDashboard(db))

	r.GET("/list_agents", getAgentsList(db))
	r.GET("/agent/:pubkey", getAgent(db))

	r.GET("/add_agent", getCreateAgentPage())
	r.POST("/add_agent", postCreateAgent(db))
	r.POST("/remove_agent", postRemoveAgent(db))

	r.GET("/change_password", getChangePassword(db))
	r.POST("/change_password", postChangePassword(db))

	r.GET("/list_users", getUsersList(db))

	r.GET("/create_user", getCreateUsersPage())
	r.POST("/create_user", postCreateUser(db))
	r.POST("/remove_user", postRemoveUser(db))

	r.GET("/notification_settings", getNotificationsConfigPage(db))
	r.POST("/notification_settings", postNotificationConfigPage(db))

	r.POST("/set_alert", postSetAlert(db))

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: CSRF(r),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %s\n", err)
		}
	}()
}

func index(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if valid, _ := checkCookie(c, db); valid {
			c.Redirect(302, "/dashboard")
			return
		}
		c.Header("Cache-Control", "no-store")
		c.HTML(http.StatusOK, "login.templ.html", gin.H{csrf.TemplateTag: csrf.TemplateField(c.Request)})
	}
}

func getDashboard(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {

		var agents []models.Agent
		if err := db.Debug().Find(&agents).Error; err != nil {
			log.Println("Loading all agents failed: ", err)
			c.String(500, "Loading failed", nil)
			return
		}

		degraded := 0
		up := 0
		var downAgents []models.Agent
		var failedEndpoints []models.MonitorEntry
		for _, agent := range agents {
			if !agent.CurrentlyConnected {
				downAgents = append(downAgents, agent)
				continue
			}

			var agentFailedEndPoints []models.MonitorEntry
			if err := db.Find(&agentFailedEndPoints, "ok = ?", false).Error; err != nil {
				log.Println("Loading all failed endpoints failed: ", err)
				c.String(500, "Endpoint loading failed", nil)
				return
			}

			if len(agentFailedEndPoints) > 0 {
				failedEndpoints = append(failedEndpoints, agentFailedEndPoints...)
				degraded++
				continue
			}

			up++

		}

		log.Println(downAgents)
		c.HTML(http.StatusOK, "dashboard.templ.html", gin.H{
			"Total":           len(agents),
			"Up":              up,
			"Down":            len(downAgents),
			"Degraded":        degraded,
			"OfflineAgents":   downAgents,
			"FailedEndpoints": failedEndpoints,
			"Agents":          agents,
		})
	}
}

func getAgentsList(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := c.Request.URL.Query()
		var agents []models.Agent

		tx := db

		if len(query["status"]) == 1 {
			status := query["status"][0]
			tx = tx.Where("currently_connected = ?", status == "online")
		}

		if err := tx.Preload("Monitors").Preload("Disks").Find(&agents).Limit(100).Error; err != nil {
			log.Println("Error getting data")
			c.String(500, "Shits broke", nil)
			return
		}

		c.HTML(http.StatusOK, "agentlist.templ.html", gin.H{"Agents": agents, csrf.TemplateTag: csrf.TemplateField(c.Request)})
	}
}

func getAgent(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {

		key, err := hex.DecodeString(c.Param("pubkey"))
		if err != nil {
			log.Println(err)
			c.String(404, "Not found nerd")

			return
		}

		var currentAgent models.Agent

		if err := db.Preload("AlertProfile").
			Preload("Monitors").
			Preload("Disks").
			Preload("SystemInfo").
			Find(&currentAgent, "pub_key = ?", string(key)).Error; err != nil {

			log.Println(err)
			c.String(404, "Not found nerd1")

			return
		}

		c.HTML(http.StatusOK, "agent.templ.html", gin.H{
			"Agent":          &currentAgent,
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})
	}
}

func getChangePassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "changepassword.templ.html", gin.H{
			"Status":         "",
			"Error":          false,
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})
	}
}

func postChangePassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := c.Keys["user"].(models.User)
		newPassword := c.PostForm("password")

		if newPassword != c.PostForm("confirmPassword") {
			log.Println("Passwords not equal")
			c.HTML(http.StatusBadRequest, "changepassword.templ.html", gin.H{
				"Status":         "Passwords do not match",
				"Error":          true,
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(c.PostForm("currentPassword"))); err != nil {
			log.Println(err)

			c.HTML(http.StatusUnauthorized, "changepassword.templ.html", gin.H{
				"Status":         "Previous password not correct",
				"Error":          true,
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			log.Println(err)
			c.String(500, "Error generating hash", nil)
			return
		}

		if err := db.Model(&u).Update("password", string(hash)).Error; err != nil {
			log.Println(err)
			c.String(500, "Error updating password", nil)
			return
		}

		c.HTML(http.StatusOK, "changepassword.templ.html", gin.H{
			"Status":         "Password changed",
			"Error":          false,
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})
	}
}

func getUsersList(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var users []models.User
		if err := db.Find(&users).Error; err != nil {
			log.Println("Cant load users: ", err)
		}
		c.Header("Cache-Control", "no-store")
		c.HTML(http.StatusOK, "userlist.templ.html", gin.H{
			"Users":          users,
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})
	}
}

func postRemoveUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {

		if err := db.Delete(&models.User{}, "guid = ?", c.PostForm("userid")).Error; err != nil {
			c.String(404, "Not found")
			return
		}

		c.Redirect(302, "/list_users")
	}
}

func getCreateUsersPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "createuser.templ.html", gin.H{
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})
	}
}

func postCreateUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.PostForm("username")
		password := c.PostForm("password")

		if len(username) == 0 || len(password) == 0 {
			c.Redirect(301, "/create_user")
			return
		}

		if err := utils.AddUser(db, username, password); err != nil {
			log.Println(err)
			c.Redirect(301, "/create_user")
			return
		}
		c.Redirect(http.StatusFound, "/list_users")
	}
}

func getCreateAgentPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "createagent.templ.html", gin.H{
			"Error":          false,
			"Status":         "",
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})
	}
}

func postCreateAgent(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {

		key := strings.TrimSpace(c.PostForm("sshkey"))
		name := strings.TrimSpace(c.PostForm("name"))

		if len(name) > 1000 {
			c.HTML(http.StatusOK, "createagent.templ.html", gin.H{
				"Error":          true,
				"Status":         "Name is too long",
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		if len(key) == 0 {
			c.HTML(http.StatusOK, "createagent.templ.html", gin.H{
				"Error":          true,
				"Status":         "SSH Key is a mandatory Field",
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			c.HTML(http.StatusOK, "createagent.templ.html", gin.H{
				"Error":          true,
				"Status":         "SSH Key is invalid",
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		var newAgent models.Agent
		newAgent.Name = name
		newAgent.PubKey = key

		if err := db.Create(&newAgent).Error; err != nil {
			log.Println("Unable to add to database: ", err)
			c.HTML(http.StatusOK, "createagent.templ.html", gin.H{
				"Error":          true,
				"Status":         "Unable to create agent",
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		c.HTML(http.StatusOK, "createagent.templ.html", gin.H{
			"Error":          false,
			"Status":         "Agent key added!",
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})

	}
}

func postRemoveAgent(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.PostForm("pubkey"))

		var toRemove models.Agent
		if err := db.Find(&toRemove, "pub_key = ?", key).Error; err != nil {
			log.Println("Unable to remove to database: ", err)
			c.Redirect(302, "/agent_list")
			return
		}

		db.Where("id = ?", toRemove.Id).Delete(&toRemove)
		db.Delete(&models.MonitorEntry{}, "agent_id = ?", toRemove.Id)
		db.Delete(&models.DiskEntry{}, "agent_id = ?", toRemove.Id)
		db.Delete(&models.Alert{}, "agent_id = ?", toRemove.Id)
		db.Delete(&models.Event{}, "agent_id = ?", toRemove.Id)
		c.Redirect(302, "/agent_list")

	}
}

func getNotificationsConfigPage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := c.Keys["user"].(models.User)

		var emailInformation models.NotificationDetail
		if err := db.Find(&emailInformation, "user_id = ?", u.Id).Error; err != nil && err != gorm.ErrRecordNotFound {
			log.Println(err)
			c.String(500, "Something went wrong with that database yo", nil)
			return
		}

		c.HTML(http.StatusOK, "notificationsettings.templ.html", gin.H{
			"Host":             emailInformation.EmailProviderHost,
			"DestinationEmail": emailInformation.Destination,
			"SendingEmail":     emailInformation.SendAddress,
			csrf.TemplateTag:   csrf.TemplateField(c.Request),
		})
	}
}

func postNotificationConfigPage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := c.Keys["user"].(models.User)

		dest := c.PostForm("destinationEmail")
		host := c.PostForm("host")
		sendAddress := c.PostForm("sendFrom")
		password := c.PostForm("sendPassword")

		if len(dest) == 0 || len(host) == 0 || len(sendAddress) == 0 || len(password) == 0 {
			c.HTML(http.StatusOK, "notificationsettings.templ.html", gin.H{
				"Status":         "All fields are manditory",
				"Error":          true,
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		_, err := mail.ParseAddress(dest)
		if err != nil {
			c.HTML(http.StatusOK, "notificationsettings.templ.html", gin.H{
				"Status":         "The destination email input was not an email address",
				"Error":          true,
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		_, err = mail.ParseAddress(sendAddress)
		if err != nil {
			c.HTML(http.StatusOK, "notificationsettings.templ.html", gin.H{
				"Status":         "The sending email input was not an email address",
				"Error":          true,
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
			return
		}

		newAlert := models.NotificationDetail{
			UserId:            u.Id,
			Destination:       dest,
			EmailProviderHost: host,
			SendAddress:       sendAddress,
			AccountPassword:   password,
		}

		var previousAlertDetails models.NotificationDetail
		if err := db.Find(&previousAlertDetails, "user_id = ?", u.Id).Error; err != nil && err != gorm.ErrRecordNotFound {

			log.Println(err)
			c.String(500, "A database error occured finding record", "")
			return
		}

		newAlert.Id = previousAlertDetails.Id

		if err := db.Save(&newAlert).Error; err != nil {
			log.Println(err)
			c.String(500, "A database error occured while saving new record", "")
			return
		}

		c.HTML(http.StatusOK, "notificationsettings.templ.html", gin.H{
			"Status":         "Information saved!",
			"Error":          false,
			csrf.TemplateTag: csrf.TemplateField(c.Request),
		})
	}
}

func postSetAlert(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pubkey := c.PostForm("pubkey")

		status := strings.TrimSpace(c.PostForm("shouldAlert"))
		diskUtil := c.PostForm("diskUtilisation")

		if len(pubkey) == 0 || len(diskUtil) == 0 {
			log.Println("Public key, or disk ultisiation percent were blank")
			c.String(400, "No length = bad length")
			return
		}

		diskInt, err := strconv.ParseInt(diskUtil, 10, 8)
		if err != nil {
			log.Println(err)
			c.String(400, "No convert = bad")
			return
		}

		decodedPubKey, err := hex.DecodeString(pubkey)
		if err != nil {
			log.Println(err)
			c.String(400, "No decode? Bad")
			return
		}

		var agent models.Agent
		if err := db.Find(&agent, "pub_key = ?", string(decodedPubKey)).Error; err != nil {
			log.Println(err)
			c.String(400, "No record = bad")
			return
		}

		newAlert := models.Alert{
			AgentId:  agent.Id,
			DiskUtil: diskInt,
			Active:   (status == "enabled"),
		}

		var alertID []int64
		if err := db.Find(&models.Alert{}, "agent_id = ?", agent.Id).Pluck("id", &alertID).Error; err == nil {
			newAlert.Id = alertID[0]
		}

		if err := db.Debug().Save(&newAlert).Error; err != nil {
			log.Println(err)
			c.String(500, "DB error, my bad")
			return
		}

		c.Redirect(302, "/agent/"+pubkey)
	}
}
