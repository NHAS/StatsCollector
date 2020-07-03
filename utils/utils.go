package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"

	"github.com/NHAS/StatsCollector/models"
	"github.com/jinzhu/gorm"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
)

func Check(message string, err error) {
	if err != nil {
		log.Fatalln(message, " : ", err)
	}
}

func AddUser(db *gorm.DB, name, password string) error {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	guid, err := GenerateHexToken(16)
	if err != nil {
		return err
	}

	newUser := &models.User{Username: name, Password: string(hashBytes), GUID: guid}

	return db.Debug().Create(newUser).Error
}

func HexFingerprintSHA256(pubKey ssh.PublicKey) string {
	sha256sum := sha256.Sum256(pubKey.Marshal())
	return hex.EncodeToString(sha256sum[:])
}

// GenerateRandomBytes returns securely generated random bytes.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return nil, err
	}

	return b, nil
}

func GenerateHexToken(n int) (string, error) {
	tokenBytes, err := GenerateRandomBytes(n)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(tokenBytes), nil
}
