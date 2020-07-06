package models

import (
	"errors"
	"log"

	"github.com/NHAS/StatsCollector/utils"
	"golang.org/x/crypto/bcrypt"
)

var ErrConfirmPasswordNotEqual = errors.New("The new and confirm passwords were not equal")
var ErrPasswordNotEqual = errors.New("The previous password did not match.")
var ErrUsernameEmpty = errors.New("Username was empty")
var ErrPasswordEmpty = errors.New("Password was empty")
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

func GetAllUsers() (users []User, err error) {
	return users, db.Find(&users).Error
}

func DeleteUser(guid string) error {
	log.Println("User: '", guid, "'")
	return db.Delete(&User{}, "guid = ?", guid).Error
}
