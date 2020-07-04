package main

import (
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/NHAS/StatsCollector/internal/iris"
	"github.com/NHAS/StatsCollector/utils"
)

func main() {

	var configPath = flag.String("config", "config.json", "Configuration file")
	var logFilePath = flag.String("log", "log.txt", "Path to log file")

	flag.Parse()

	logFile, err := os.OpenFile(*logFilePath, os.O_CREATE|os.O_RDWR, 0600)
	utils.Check("Opening logging file failed", err)

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	flagset := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { flagset[f.Name] = true })

	configurationBytes, err := ioutil.ReadFile(*configPath)
	utils.Check("Load [settings] failed", err)

	var config iris.ClientConfig

	config.UpdateIntervalSec = 240
	err = json.Unmarshal(configurationBytes, &config)
	utils.Check("Unmarshalling [settings[ failed", err)

	iris.RunClient(config)
}
