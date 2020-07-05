package webservice

import (
	"log"
	"net/http"
	"time"

	"github.com/NHAS/StatsCollector/models"
	"github.com/NHAS/StatsCollector/utils"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	"golang.org/x/crypto/bcrypt"
)

func setupSessionRoutes(r *gin.Engine, db *gorm.DB) {

	b, err := utils.GenerateRandomBytes(16)
	utils.Check("Generating random bytes failed", err)

	dummyPassword, err := bcrypt.GenerateFromPassword(b, bcrypt.DefaultCost)
	utils.Check("Creating dummy password hash failed", err)

	r.POST("/authenticate", authenticatePOST(db, dummyPassword))
	r.GET("/logout", logoutGET(db))

}

func authenticatePOST(db *gorm.DB, dummyPassword []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.PostForm("username")
		password := c.PostForm("password")

		if len(username) == 0 || len(password) == 0 {
			c.Redirect(302, "/")
			return
		}

		var record models.User
		if err := db.Where("username = ?", username).First(&record).Error; err != nil {
			bcrypt.CompareHashAndPassword(dummyPassword, []byte(password)) // Dummy compair to stop timing attacks
			c.Redirect(302, "/")
			log.Println(err)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(record.Password), []byte(password)); err != nil {
			c.Redirect(302, "/")
			log.Println(err)
			return
		}

		password = "" // Clear password from memory asap

		token, err := utils.GenerateHexToken(TokenSize)
		if err != nil {
			log.Println("Error generating token: ", err)
			c.String(http.StatusInternalServerError, "Server error")
			return
		}

		if db.Model(&record).Updates(models.User{
			Token:          token,
			TokenCreatedAt: time.Now().Unix(),
		}).Error != nil {
			log.Println("Error saving token in database: ", err)
			c.String(http.StatusInternalServerError, "Server error")
			return
		}

		c.SetSameSite(http.SameSiteStrictMode) // Stupid way of setting same site gin....
		c.SetCookie(CookieName, record.Username+":"+token, 3600, "", "localhost:8080", false, true)

		c.Redirect(302, "/dashboard/")

	}

}

func logoutGET(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		valid, u := checkCookie(c, db)
		if !valid {
			denyRequest(c)
			return
		}

		newToken, err := utils.GenerateHexToken(TokenSize)
		if err != nil {
			log.Println("Error generating random bytes for token: ", err)
			c.String(http.StatusInternalServerError, "Server error")
			return
		}

		if err := db.Debug().Model(&models.User{}).Where("guid = ? AND token = ?", u.GUID, u.Token).
			Updates(models.User{
				Token:          newToken,
				TokenCreatedAt: time.Now().Unix(),
			}).Error; err != nil {
			log.Println("Error saving token in database: ", err)
			c.String(http.StatusInternalServerError, "Server error")
			return
		}

		c.Redirect(302, "/")

	}
}
