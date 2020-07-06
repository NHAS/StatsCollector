package webservice

import (
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/NHAS/StatsCollector/models"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/jinzhu/gorm"
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

		totalAgents, downAgents, degradedAgents, failedEndPoints, err := models.GetDashboardInformation()
		if err != nil {
			log.Println("Unable to load information for dashboard: ", err)
			c.String(500, "Unable to load dashboard")
			return
		}

		c.HTML(http.StatusOK, "dashboard.templ.html", gin.H{
			"Total":           totalAgents,
			"Up":              totalAgents - len(downAgents) - len(degradedAgents),
			"Down":            len(downAgents),
			"Degraded":        len(degradedAgents),
			"OfflineAgents":   downAgents,
			"FailedEndpoints": failedEndPoints,
		})
	}
}

func getAgentsList(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := c.Request.URL.Query()

		filter := ""
		if len(query["status"]) == 1 {
			filter = query["status"][0]
		}

		agents, err := models.GetAgentList(filter, 100)
		if err != nil {
			log.Println("Error getting agents list: ", err)
			c.String(500, "Unable to get agents list")

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

		currentAgent, err := models.GetAgent(string(key))
		if err != nil {
			log.Println("Unable to get current agent: ", err)
			c.String(404, "Agent not found")
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
		confirm := c.PostForm("confirmPassword")
		previousPassword := c.PostForm("currentPassword")

		if err := models.ChangePassword(u.Id, newPassword, confirm, previousPassword, u.Password); err != nil {
			c.HTML(http.StatusOK, "changepassword.templ.html", gin.H{
				"Status":         err.Error(),
				"Error":          true,
				csrf.TemplateTag: csrf.TemplateField(c.Request),
			})
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

		users, err := models.GetAllUsers()
		if err != nil {
			c.String(500, err.Error())
			return
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

		err := models.DeleteUser(c.PostForm("userid"))
		if err != nil {
			c.String(500, err.Error())
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

		if err := models.AddUser(username, password); err != nil {
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
		name := strings.TrimSpace(c.PostForm("name"))
		key := strings.TrimSpace(c.PostForm("sshkey"))

		err := models.CreateAgent(name, key)
		if err != nil {
			c.HTML(http.StatusOK, "createagent.templ.html", gin.H{
				"Error":          true,
				"Status":         err.Error(),
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

		err := models.DeleteAgent(key)
		if err != nil {
			log.Println("Error removing agent: ", err)
		}
		c.Redirect(302, "/list_agents")

	}
}

func getNotificationsConfigPage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := c.Keys["user"].(models.User)

		emailInformation, err := models.GetNotificationSettingsForUser(u.Id)
		if err != nil && err != gorm.ErrRecordNotFound {
			c.String(500, "Error fetching data")
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

		emailInformation, err := models.GetNotificationSettingsForUser(u.Id)
		if err != nil && err != gorm.ErrRecordNotFound {
			c.String(500, "Error fetching data")
			return
		}

		err = models.CreateNotificationSetting(u.Id, dest, sendAddress, password, host)
		if err != nil {
			c.HTML(http.StatusOK, "notificationsettings.templ.html", gin.H{
				"Host":             emailInformation.EmailProviderHost,
				"DestinationEmail": emailInformation.Destination,
				"SendingEmail":     emailInformation.SendAddress,
				"Status":           err.Error(),
				"Error":            true,
				csrf.TemplateTag:   csrf.TemplateField(c.Request),
			})
			return
		}

		c.HTML(http.StatusOK, "notificationsettings.templ.html", gin.H{
			"Host":             emailInformation.EmailProviderHost,
			"DestinationEmail": emailInformation.Destination,
			"SendingEmail":     emailInformation.SendAddress,
			"Status":           "Information saved!",
			"Error":            false,
			csrf.TemplateTag:   csrf.TemplateField(c.Request),
		})
	}
}

func postSetAlert(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {

		status := strings.TrimSpace(c.PostForm("shouldAlert"))
		diskUtil := c.PostForm("diskUtilisation")

		diskInt, err := strconv.ParseInt(diskUtil, 10, 8)
		if err != nil {
			log.Println(err)
			c.String(400, "No convert = bad")
			return
		}

		pubkey, err := hex.DecodeString(c.PostForm("pubkey"))
		if err != nil {
			log.Println(err)
			c.String(400, "No decode? Bad")
			return
		}

		err = models.CreateAlertProfileForAgent(string(pubkey), diskInt, (status == "enabled"))
		if err != nil {
			c.String(500, err.Error())
			return
		}

		c.Redirect(302, "/agent/"+c.PostForm("pubkey"))
	}
}
