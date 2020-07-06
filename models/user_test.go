package models

import (
	"log"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"golang.org/x/crypto/bcrypt"
)

func setupDatabase() {
	dbSqlite, err := gorm.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Println(err)
	}

	InitaliseModels(dbSqlite)

}

func TestAddUserValid(t *testing.T) {
	setupDatabase()
	defer db.Close()

	pwd := "test"

	err := AddUser("test", pwd)
	if err != nil {
		t.Fatal(err)
	}

	var user User
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
