package iris

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/NHAS/StatsCollector/models"
	"github.com/NHAS/StatsCollector/utils"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	"golang.org/x/crypto/ssh"
)

type monitor struct {
	URL            string `json:"url"`
	OkayCode       int    `json:"okay_code"`
	OkayString     string `json:"okay_string"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type ClientConfig struct {
	ServerAddress     string    `json:"server_address"`
	AuthorisedKey     string    `json:"authorised_key"`
	MonitorURLS       []monitor `json:"monitor_urls"`
	PrivateKeyPath    string    `json:"private_key_path"`
	UpdateIntervalSec int       `json:"update_seconds"`
}

func RunClient(config ClientConfig) {

	hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(config.AuthorisedKey))
	utils.Check("Parse [authorised key] failed", err)

	privateBytes, err := ioutil.ReadFile(config.PrivateKeyPath)
	utils.Check("Load [private key] failed", err)

	private, err := ssh.ParsePrivateKey(privateBytes)
	utils.Check("Parse [private key] failed", err)

	// An SSH client is represented with a ClientConn.
	//
	// To authenticate with the remote server you must pass at least one
	// implementation of AuthMethod via the Auth field in ClientConfig,
	// and provide a HostKeyCallback.
	sshConfig := &ssh.ClientConfig{
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(private),
		},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}

	for {

		con, err := net.Dial("tcp", config.ServerAddress)
		if err != nil {
			log.Println("Connecting to stats server failed", err)
			log.Println("Attempting to reconnect after 20 seconds")
			<-time.After(20 * time.Second)
			continue
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(con, config.ServerAddress, sshConfig)
		if err != nil {
			log.Println("Starting ssh client connection failed", err)
			log.Println("Attempting to reconnect after 20 seconds")
			<-time.After(20 * time.Second)
			continue
		}
		defer sshConn.Close()

		// The incoming Request channel must be serviced.
		go ssh.DiscardRequests(reqs)

		channel, reqs, err := sshConn.OpenChannel("metrics", nil)
		utils.Check("Opening metrics channel failed", err)

		go ssh.DiscardRequests(reqs)

		go func() {
			log.Println("Started sending system info")

			for {
				systemInfoBytes, err := getSystemInfo()
				if err != nil {
					log.Println("Unable to send system info: ", err)
					return
				}

				channel.SendRequest("system", false, systemInfoBytes)

				<-time.After(1 * time.Hour)
			}
		}()

		go func() {
			defer channel.Close()

			log.Println("Started sending updates")
			for {

				contents, err := getStats(config.MonitorURLS)
				utils.Check("Failed to get stats", err)

				_, err = channel.Write(contents)
				if err != nil {
					log.Println("Writing to channel failed", err)
					return
				}
				<-time.After(time.Duration(config.UpdateIntervalSec) * time.Second)
			}
		}()

		for newChannel := range chans {
			newChannel.Reject(ssh.Prohibited, "Clients disallow channel requests")
		}

	}

}

func getStats(monitorUrls []monitor) ([]byte, error) {
	monitorsStatus := make(chan []models.MonitorStatus)
	quit := make(chan bool)

	go checkMonitors(monitorUrls, monitorsStatus, quit)

	disksUsedPercent, err := getDisks()
	if err != nil {
		quit <- true
		return []byte(""), err
	}

	memUsedPercent, err := getMemory()
	if err != nil {
		quit <- true
		return []byte(""), err
	}

	mons := <-monitorsStatus

	stat := &models.Stats{
		DiskUsage:     disksUsedPercent,
		MemoryUsage:   memUsedPercent,
		MonitorValues: mons,
	}

	return json.Marshal(stat)
}

func checkMonitors(monitorUrls []monitor, final chan<- []models.MonitorStatus, end <-chan bool) {

	output := make([]models.MonitorStatus, len(monitorUrls))

check:
	for index, m := range monitorUrls {

		select {
		case <-end:
			break check
		default:

		}

		ms := models.MonitorStatus{Path: m.URL, Reason: "-", OK: true}

		u, err := url.Parse(m.URL)
		if err != nil {
			log.Println("Warning, was unable to parse URL:", m.URL, " Err:", err, "Skipping")
			ms.OK = false
			ms.Reason = "Unable to parse URL: " + err.Error()
			output[index] = ms
			continue
		}

		switch strings.TrimSpace(u.Scheme) {

		case "http", "https":

			httpClient := http.Client{
				Timeout: time.Duration(m.TimeoutSeconds) * time.Second,
			}

			resp, err := httpClient.Get(m.URL)
			if err != nil {
				ms.OK = false
				ms.Reason = "HTTP get failed: " + err.Error()
				output[index] = ms
				continue
			}

			ms.StatusCode = resp.StatusCode

			if resp.StatusCode != m.OkayCode {
				ms.OK = false
				ms.Reason = "Status code not expected"
				output[index] = ms
				continue
			}

			defer resp.Body.Close()

			if len(m.OkayString) > 0 {

				contents, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					ms.OK = false
					ms.Reason = "Unable to read body"
					output[index] = ms
					continue
				}

				if !bytes.Contains(contents, []byte(m.OkayString)) {
					ms.OK = false
					ms.Reason = "String not found"
					output[index] = ms
					continue
				}
			}

		default:
			d := net.Dialer{Timeout: time.Duration(m.TimeoutSeconds) * time.Second}
			_, err := d.Dial(u.Scheme, u.Host)
			if err != nil {
				ms.OK = false
				ms.Reason = "Could not connect: " + err.Error()
				output[index] = ms
				continue
			}
		}

		output[index] = ms
	}

	final <- output
}

func getMemory() (float32, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}

	return float32(v.UsedPercent), nil
}

func getDisks() (d map[string]float32, err error) {
	d = make(map[string]float32)

	partitions, err := disk.Partitions(false)
	if err != nil {
		return d, err
	}

	for _, currentDisk := range partitions {
		if !strings.HasPrefix(currentDisk.Device, "/") {
			continue
		}

		if _, ok := d[currentDisk.Device]; !ok {

			usage, err := disk.Usage(currentDisk.Mountpoint)
			if err == nil {
				d[currentDisk.Device] = float32(usage.UsedPercent)
			} else {
				d[currentDisk.Device] = -1
				log.Println("[", currentDisk.Mountpoint, "] Warning: ", err)
			}
		}
	}

	return d, err
}

func getSystemInfo() ([]byte, error) {
	cores, err := cpu.Counts(false)
	if err != nil {
		return []byte(""), err
	}

	m, err := mem.VirtualMemory()
	if err != nil {
		return []byte(""), err
	}

	platform, family, version, err := host.PlatformInformation()
	if err != nil {
		return []byte(""), err
	}

	output := &models.SystemInfo{
		CpuCores:    cores,
		TotalMemory: m.Total,
		Platform:    platform,
		Family:      family,
		Version:     version,
	}

	return json.Marshal(output)
}
