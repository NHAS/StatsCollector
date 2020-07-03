package models

// User is the structure serialised into the database that holds all user information
type User struct {
	Id             int64  `form:"-"`
	GUID           string `form:"-" gorm:"unique;not null"`
	Username       string `form:"username" binding:"required" gorm:"unique;not null"`
	Password       string `form:"password" binding:"required" gorm:"unique;not null"`
	Token          string `form:"-" gorm:"unique;"`
	TokenCreatedAt int64  `form:"-"`

	NotificationInformation NotificationDetail
}
