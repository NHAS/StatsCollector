package models

import (
	"errors"
	"log"

	"github.com/NHAS/StatsCollector/utils"
	"golang.org/x/crypto/bcrypt"
)

//ErrConfirmPasswordNotEqual should be returned if the user entered in two different passwords on the "confirm" and "new password" fields on the web interface
var ErrConfirmPasswordNotEqual = errors.New("The new and confirm passwords were not equal")

//ErrPasswordNotEqual if the "current password" is not equal to what the user just submitted (To stop account take over)
var ErrPasswordNotEqual = errors.New("The previous password did not match")

// ErrUsernameEmpty is returned if during new user creation the username was not specified
var ErrUsernameEmpty = errors.New("Username was empty")

//ErrPasswordEmpty same as above, but for password
var ErrPasswordEmpty = errors.New("Password was empty")

//ErrPasswordTooShort is returned If the password to be set is smaller than 10 characters deny it with this error
var ErrPasswordTooShort = errors.New("Password was below 10 characters in length")

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

//AddUser adds a user to the database if the information supplied is valid
func AddUser(name, password string) error {
	if len(name) == 0 {
		return ErrUsernameEmpty
	}

	if len(password) == 0 {
		return ErrPasswordEmpty
	}

	if len(password) < 10 {
		return ErrPasswordTooShort
	}

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	guid, err := utils.GenerateHexToken(16)
	if err != nil {
		return err
	}

	newUser := &User{Username: name, Password: string(hashBytes), GUID: guid}

	return db.Debug().Create(newUser).Error
}

//ChangePassword change password for user, which checks if the user knows the currrent password.
func ChangePassword(uid int64, newPassword, confirmNewPassword, currentPasswordInput, currentPasswordHash string) error {

	if newPassword != confirmNewPassword {
		return ErrConfirmPasswordNotEqual
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentPasswordHash), []byte(currentPasswordInput)); err != nil {
		return ErrPasswordNotEqual
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Println(err)

		return err
	}

	if err := db.Model(&User{}).Where("id = ?", uid).Update("password", string(hash)).Error; err != nil {
		log.Println(err)
		return err
	}

	return nil
}

//GetAllUsers is a function that returns a list of all users registered
func GetAllUsers() (users []User, err error) {
	return users, db.Find(&users).Error
}

//DeleteUser is the function which deletes users
func DeleteUser(guid string) error {
	log.Println("User: '", guid, "'")
	return db.Delete(&User{}, "guid = ?", guid).Error
}
