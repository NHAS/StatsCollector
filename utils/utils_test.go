package utils

import (
	"log"
	"testing"

	"github.com/NHAS/StatsCollector/models"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"golang.org/x/crypto/bcrypt"
)

func setupDatabase() *gorm.DB {
	db, err := gorm.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Println(err)
	}
	models.InitaliseModels(db)
	return db
}

func TestAddUserValid(t *testing.T) {
	db := setupDatabase()
	defer db.Close()

	pwd := "test"

	err := AddUser(db, "test", pwd)
	if err != nil {
		t.Fatal(err)
	}

	var user models.User
	if err := db.Debug().Find(&user, "username = ?", "test").Error; err != nil {
		t.Fatal(err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(pwd))
	if err != nil {
		t.Fatal(err)
	}

	if user.Username != "test" {
		t.Fatal("Username does not equal set username")
	}
}
