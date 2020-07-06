package models

import (
	"StatsCollector/models"
	"errors"
	"net/mail"
	"time"

	"github.com/jinzhu/gorm"
)

//NotificationDetail is a structure to store user notification preferences in the database
type NotificationDetail struct {
	Id        int64
	UserId    int64
	UpdatedAt time.Time

	Destination       string
	SendAddress       string
	AccountPassword   string
	EmailProviderHost string
}

var ErrManditoryFieldsNotFilled = errors.New("Manditory fields were not filled with data")
var ErrNotValidEmailAddress = errors.New("Not a valid email address")

func GetNotificationSettingsForUser(uid int64) (emailInformation NotificationDetail, err error) {
	return emailInformation, db.Find(&emailInformation, "user_id = ?", uid).Error
}

func CreateNotificationSetting(uid int64, destiniationEmail, sendingAccountEmail, sendingAccountPassword, emailProvider string) error {
	if len(destiniationEmail) == 0 || len(sendingAccountEmail) == 0 || len(sendingAccountPassword) == 0 || len(emailProvider) == 0 {
		return ErrManditoryFieldsNotFilled
	}

	_, err := mail.ParseAddress(destiniationEmail)
	if err != nil {
		return ErrNotValidEmailAddress
	}

	_, err = mail.ParseAddress(sendingAccountEmail)
	if err != nil {
		return ErrNotValidEmailAddress
	}

	newProfile := NotificationDetail{
		UserId:            uid,
		Destination:       destiniationEmail,
		EmailProviderHost: emailProvider,
		SendAddress:       sendingAccountEmail,
		AccountPassword:   sendingAccountPassword,
	}

	var previousAlertDetails models.NotificationDetail
	if err := db.Debug().Find(&previousAlertDetails, "user_id = ?", uid).Error; err != nil && err != gorm.ErrRecordNotFound {

		return err
	}

	newProfile.Id = previousAlertDetails.Id

	return db.Debug().Save(&newProfile).Error
}
