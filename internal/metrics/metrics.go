package metrics

import (
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ActiveRooms tracks current number of active WebRTC rooms
	ActiveRooms = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "sfu_active_rooms",
		Help: "Current number of active WebRTC rooms",
	})

	// ActiveParticipants tracks current number of connected participants (hosts, cohosts, viewers)
	ActiveParticipants = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "sfu_active_participants",
		Help: "Current number of active participants in the SFU",
	})

	// BytesReceivedTotal tracks total bandwidth bytes received into the SFU (inbound RTP/signaling)
	BytesReceivedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sfu_bytes_received_total",
		Help: "Total number of bytes received by the SFU",
	})

	// BytesSentTotal tracks total bandwidth bytes transmitted from the SFU (outbound RTP/signaling)
	BytesSentTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sfu_bytes_sent_total",
		Help: "Total number of bytes sent by the SFU",
	})
)

// IncActiveRooms increments the sfu_active_rooms gauge
func IncActiveRooms() {
	ActiveRooms.Inc()
}

// DecActiveRooms decrements the sfu_active_rooms gauge
func DecActiveRooms() {
	ActiveRooms.Dec()
}

// IncActiveParticipants increments the sfu_active_participants gauge
func IncActiveParticipants() {
	ActiveParticipants.Inc()
}

// DecActiveParticipants decrements the sfu_active_participants gauge
func DecActiveParticipants() {
	ActiveParticipants.Dec()
}

// AddBytesReceived adds n bytes to the sfu_bytes_received_total counter
func AddBytesReceived(n int) {
	if n > 0 {
		BytesReceivedTotal.Add(float64(n))
	}
}

// AddBytesSent adds n bytes to the sfu_bytes_sent_total counter
func AddBytesSent(n int) {
	if n > 0 {
		BytesSentTotal.Add(float64(n))
	}
}

// SysSummary represents CPU, Goroutine and Memory statistics for health & load balancers
type SysSummary struct {
	NumCPU        int    `json:"num_cpu"`
	NumGoroutine  int    `json:"num_goroutine"`
	AllocMB       uint64 `json:"alloc_mb"`
	TotalAllocMB  uint64 `json:"total_alloc_mb"`
	SysMB         uint64 `json:"sys_mb"`
	NumGC         uint32 `json:"num_gc"`
	LiveObjects   uint64 `json:"live_objects"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

var startTime = time.Now()

// GetSystemSummary gathers current runtime memory and CPU stats
func GetSystemSummary() SysSummary {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SysSummary{
		NumCPU:        runtime.NumCPU(),
		NumGoroutine:  runtime.NumGoroutine(),
		AllocMB:       m.Alloc / 1024 / 1024,
		TotalAllocMB:  m.TotalAlloc / 1024 / 1024,
		SysMB:         m.Sys / 1024 / 1024,
		NumGC:         m.NumGC,
		LiveObjects:   m.Mallocs - m.Frees,
		UptimeSeconds: int64(time.Since(startTime).Seconds()),
	}
}
