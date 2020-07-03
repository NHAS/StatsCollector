package models

// Stats is the big object that is passed around through ssh to give system metrics
type Stats struct {
	MonitorValues []MonitorStatus

	DiskUsage   map[string]float32
	MemoryUsage float32
}
