package webservice

import (
	"strings"
	"time"

	"github.com/NHAS/StatsCollector/models"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
)

func authorisionMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		valid, user := checkCookie(c, db)
		if !valid {
			denyRequest(c)
			return
		}
		c.Keys = make(map[string]interface{})
		c.Keys["user"] = user
	}
}

func denyRequest(c *gin.Context) {
	c.Redirect(302, "/")
	c.Abort()
}

func checkCookie(c *gin.Context, db *gorm.DB) (valid bool, u models.User) {
	contents, err := c.Cookie(CookieName)
	if err != nil {
		return false, u
	}

	parts := strings.Split(contents, ":")
	if len(parts) != 2 {
		return false, u
	}

	var record models.User
	if db.Debug().Where("username = ? AND token = ?", parts[0], parts[1]).First(&record).Error != nil {
		return false, u
	}

	expiresAt := time.Unix(record.TokenCreatedAt, 0).Add(1 * time.Hour)
	if time.Now().After(expiresAt) {
		return false, u
	}

	return true, record
}
