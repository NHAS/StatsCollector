package theia

import (
	"log"
	"testing"

	"github.com/NHAS/StatsCollector/models"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

func setupDatabase() *gorm.DB {
	db, err := gorm.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Println(err)
	}
	models.InitaliseModels(db)
	return db
}

func TestUnratelimitedSendEvent(t *testing.T) {
	db := setupDatabase()
	defer db.Close()

	title := "test 1 title"

	if err := sendEvent(db, -1, 10, title, "message"); err != nil {
		t.Fatal(err)
	}

	var event models.Event
	if err := db.First(&event, "title = ?", title).Error; err != nil {
		t.Fatal(err)
	}

	if event.AgentId != -1 || event.Title != title {
		t.Fatal("Event data wasnt returned....")
	}
}

func TestUniqueEventsUnratelimitedSendEvent(t *testing.T) {
	db := setupDatabase()
	defer db.Close()

	title := "test 1 title"

	if err := sendEvent(db, -1, 10, title, "message"); err != nil {
		t.Fatal(err)
	}

	if err := sendEvent(db, -1, 10, title+"2", "message"); err != nil {
		t.Fatal(err)
	}

	if err := sendEvent(db, -1, 10, title+"3", "message"); err != nil {
		t.Fatal(err)
	}
}

func TestRatelimitedSendEvent(t *testing.T) {
	db := setupDatabase()
	defer db.Close()

	title := "test 1 title"

	if err := sendEvent(db, -1, 10, title, "message"); err != nil {
		t.Fatal(err)
	}

	if err := sendEvent(db, -1, 10, title, "message"); err != ErrRatelimited {
		t.Fatal("Request wasnt ratelimited")
	}
}

func TestGetAgentsWithIssuesEmpty(t *testing.T) {
	db := setupDatabase()
	defer db.Close()

	agents, err := getAgentsWithIssues(db)
	if err != nil {
		t.Fatal(err)
	}

	if len(agents) > 0 {
		t.Fatal("Cant be finding agents if there are no agents present")
	}
}
