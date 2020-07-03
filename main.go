package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/NHAS/StatsCollector/models"
	"github.com/NHAS/StatsCollector/utils"
	"github.com/jinzhu/gorm"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {

	flag.Bool("server", false, "If set start server")
	flag.Bool("adduser", false, "Add user to database and quit")
	var configPath = flag.String("config", "config.json", "Configuration file")
	var logFilePath = flag.String("log", "log.txt", "Path to log file")

	flag.Parse()

	logFile, err := os.OpenFile(*logFilePath, os.O_CREATE|os.O_RDWR, 0600)
	utils.Check("Opening logging file failed", err)

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	flagset := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { flagset[f.Name] = true })

	if flagset["server"] || flagset["adduser"] {

		db, err := gorm.Open("postgres", "sslmode=disable host=localhost port=5432 user=gorm dbname=stats password=")
		utils.Check("Could not connect to database", err)

		if flagset["server"] {
			runServer(db, *configPath)
			return
		}

		db.AutoMigrate(&models.User{})
		username, password, err := credentials()
		utils.Check("Unable to get password", err)

		err = utils.AddUser(db, username, password)
		utils.Check("Unable to add user to database", err)

		log.Println("User added")
		return
	}

	runClient(*configPath)

}

func credentials() (string, string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return "", "", err
	}

	fmt.Print("Enter Password: ")
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", "", err
	}
	password := string(bytePassword)

	return strings.TrimSpace(username), strings.TrimSpace(password), nil
}
