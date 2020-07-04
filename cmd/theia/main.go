package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/NHAS/StatsCollector/internal/theia"
	"github.com/NHAS/StatsCollector/models"
	"github.com/NHAS/StatsCollector/utils"
	"github.com/jinzhu/gorm"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	flag.Bool("adduser", false, "Add user to database and quit")
	var configPath = flag.String("config", "config.json", "Configuration file")
	var logFilePath = flag.String("log", "log.txt", "Path to log file")

	flag.Parse()

	logFile, err := os.OpenFile(*logFilePath, os.O_CREATE|os.O_RDWR, 0600)
	utils.Check("Opening logging file failed", err)

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	flagset := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { flagset[f.Name] = true })

	pwd := os.Getenv("PASSWORD")

	db, err := gorm.Open("postgres", "sslmode=disable host=localhost port=5432 user=gorm dbname=stats password="+pwd)
	utils.Check("Could not connect to database", err)

	if flagset["adduser"] {

		db.AutoMigrate(&models.User{})
		username, password, err := credentials()
		utils.Check("Unable to get password", err)

		err = utils.AddUser(db, username, password)
		utils.Check("Unable to add user to database", err)

		log.Println("User added")
		return
	}

	// Public key authentication is done by comparing
	// the public key of a received connection
	// with the entries in the authorized_keys file.
	configurationBytes, err := ioutil.ReadFile(*configPath)
	utils.Check("Failed to load settings", err)

	var config theia.ServerConfig
	config.WebResourcesPath = "."
	err = json.Unmarshal(configurationBytes, &config)
	utils.Check("Failed to unmarshal config", err)

	theia.RunServer(db, config)

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
