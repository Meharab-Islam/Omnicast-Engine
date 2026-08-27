package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerYAML holds HTTP and logging configurations
type ServerYAML struct {
	Port     int    `yaml:"port"`
	Mode     string `yaml:"mode"`
	LogLevel string `yaml:"log_level"`
}

// WebRTCYAML holds SFU Simulcast, Dynacast, and ICE timeouts
type WebRTCYAML struct {
	SimulcastEnabled bool `yaml:"simulcast_enabled"`
	DynacastEnabled  bool `yaml:"dynacast_enabled"`
	MaxBitrateHigh   int  `yaml:"max_bitrate_high"`
	MaxBitrateMedium int  `yaml:"max_bitrate_medium"`
	MaxBitrateLow    int  `yaml:"max_bitrate_low"`
	ICETimeoutSec    int  `yaml:"ice_timeout_sec"`
}

// RoomManagementYAML holds participant limits and timeouts (0 = UNLIMITED)
type RoomManagementYAML struct {
	HostGracePeriodSec       int `yaml:"host_grace_period_sec"`
	EmptyRoomTimeoutSec      int `yaml:"empty_room_timeout_sec"`
	MaxViewersPerRoom        int `yaml:"max_viewers_per_room"` // 0 for UNLIMITED
	LateJoinerSyncIntervalMs int `yaml:"late_joiner_sync_interval_ms"`
}

// CoHostingYAML holds maximum active co-host seat capacity (0 = UNLIMITED)
type CoHostingYAML struct {
	MaxActiveSeats int `yaml:"max_active_seats"` // 0 for UNLIMITED
}

// PKBattleYAML holds cross-room battle durations and sync intervals
type PKBattleYAML struct {
	DefaultDurationSec int `yaml:"default_duration_sec"`
	SyncIntervalMs     int `yaml:"sync_interval_ms"`
}

// InteractionsYAML holds chat rate limits and Redis batch flusher timings
type InteractionsYAML struct {
	ChatRateLimitPerSec   int `yaml:"chat_rate_limit_per_sec"`
	GiftRedisBatchFlushMs int `yaml:"gift_redis_batch_flush_ms"`
}

// CascadingYAML holds inter-server cascading reconnect timings
type CascadingYAML struct {
	EdgeReconnectRetrySec int `yaml:"edge_reconnect_retry_sec"`
}

// ModerationYAML holds viewer reconnection grace period and auto-ban rules
type ModerationYAML struct {
	ViewerGracePeriodSec int  `yaml:"viewer_grace_period_sec"`
	AutoBanOnKick        bool `yaml:"auto_ban_on_kick"`
}

// YAMLConfig represents the full centralized config.yaml structure
type YAMLConfig struct {
	Server         ServerYAML         `yaml:"server"`
	WebRTC         WebRTCYAML         `yaml:"webrtc"`
	RoomManagement RoomManagementYAML `yaml:"room_management"`
	CoHosting      CoHostingYAML      `yaml:"co_hosting"`
	Moderation     ModerationYAML     `yaml:"moderation"`
	PKBattle       PKBattleYAML       `yaml:"pk_battle"`
	Interactions   InteractionsYAML   `yaml:"interactions"`
	Cascading      CascadingYAML      `yaml:"cascading"`
}

// DefaultYAMLContent is the exhaustive default template with UNLIMITED controls
const DefaultYAMLContent = `server:
  port: 8080
  mode: "production"
  log_level: "info"

webrtc:
  simulcast_enabled: true
  dynacast_enabled: true
  max_bitrate_high: 1200000
  max_bitrate_medium: 500000
  max_bitrate_low: 150000
  ice_timeout_sec: 15

room_management:
  host_grace_period_sec: 25           
  empty_room_timeout_sec: 300         # Time before auto-deleting an empty room. IF SET TO 0, NEVER DESTROY THE ROOM.
  max_viewers_per_room: 0             # Set to 0 for UNLIMITED participants, or specify a number (e.g., 100000)
  late_joiner_sync_interval_ms: 100   

co_hosting:
  max_active_seats: 0                 # Set to 0 for UNLIMITED co-hosts, or specify a number (e.g., 4, 8, 12)

moderation:
  viewer_grace_period_sec: 120        # How long to wait for a disconnected viewer/co-host to reconnect before kicking them (e.g., 2 mins)
  auto_ban_on_kick: true              # If Host manually kicks a user, prevent them from rejoining

pk_battle:
  default_duration_sec: 300           
  sync_interval_ms: 500               

interactions:
  chat_rate_limit_per_sec: 3          
  gift_redis_batch_flush_ms: 2000     

cascading:
  edge_reconnect_retry_sec: 5
`

// Config holds both runtime environment secrets and parsed YAML rules
type Config struct {
	Port         string
	DomainName   string
	HostDomain   string
	PublicIP     string
	APIKey       string
	APISecret    string
	JWTSecret    string
	TurnSecret   string
	TurnRealm    string
	RedisAddr    string
	RedisPass    string
	ServerRole   string
	ServerID     string
	WebhookURL   string
	DataDir      string
	EnvFilePath  string
	YAMLFilePath string

	YAML YAMLConfig
}

var (
	globalConfig *Config
	configMu     sync.RWMutex
)

// GetAppConfig returns the active global Config instance thread-safely
func GetAppConfig() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	if globalConfig == nil {
		// Fallback to default if not yet initialized
		return &Config{
			YAML: YAMLConfig{
				RoomManagement: RoomManagementYAML{MaxViewersPerRoom: 0},
				CoHosting:      CoHostingYAML{MaxActiveSeats: 0},
			},
		}
	}
	return globalConfig
}

// GenerateRandomKey produces a secure random hexadecimal string of given byte length
func GenerateRandomKey(byteLength int) string {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("sec_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// DetectPublicIP attempts to fetch the machine's external public IP using lightweight IP services
func DetectPublicIP() string {
	client := http.Client{
		Timeout: 2500 * time.Millisecond,
	}

	endpoints := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	for _, url := range endpoints {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err == nil {
				ip := strings.TrimSpace(string(body))
				if ip != "" && !strings.Contains(ip, "<") && len(ip) <= 45 {
					return ip
				}
			}
		}
	}

	return ""
}

// LoadOrGenerateConfig checks existing environment variables, auto-generates config.yaml & .env if missing,
// loads the YAML capacity rules, and returns the unified Config.
func LoadOrGenerateConfig() *Config {
	configMu.Lock()
	defer configMu.Unlock()

	cfg := &Config{}

	// 1. Determine persistent YAML config path (check ./config/config.yaml, /app/config/config.yaml, or ./config.yaml)
	possibleYAMLPaths := []string{
		"config/config.yaml",
		"/app/config/config.yaml",
		"config.yaml",
	}

	selectedYAMLPath := "config/config.yaml"
	for _, p := range possibleYAMLPaths {
		if _, err := os.Stat(p); err == nil {
			selectedYAMLPath = p
			break
		}
	}
	cfg.YAMLFilePath = selectedYAMLPath

	// Auto-generate config.yaml if it does not exist
	if _, err := os.Stat(selectedYAMLPath); os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Dir(selectedYAMLPath), 0755)
		if err := os.WriteFile(selectedYAMLPath, []byte(DefaultYAMLContent), 0644); err == nil {
			log.Printf("[Zero-Config] Auto-generated default config.yaml at %s\n", selectedYAMLPath)
		}
	}

	// Read and unmarshal config.yaml
	if yamlData, err := os.ReadFile(selectedYAMLPath); err == nil {
		var ycfg YAMLConfig
		if err := yaml.Unmarshal(yamlData, &ycfg); err == nil {
			cfg.YAML = ycfg
			log.Printf("[Config] Loaded config.yaml (Max Viewers: %d [0=Unlimited], Max Seats: %d [0=Unlimited])\n",
				ycfg.RoomManagement.MaxViewersPerRoom, ycfg.CoHosting.MaxActiveSeats)
		} else {
			log.Printf("[Config Warning] Failed to parse %s: %v. Using default rules.\n", selectedYAMLPath, err)
		}
	}

	// 2. Determine persistent .env storage path
	possiblePaths := []string{
		"/app/data/.env",
		"data/.env",
		".env",
	}

	selectedEnvPath := ".env"
	for _, p := range possiblePaths {
		dir := filepath.Dir(p)
		if dir == "." || dir == "/app/data" || dir == "data" {
			if _, err := os.Stat(dir); err == nil || dir == "." {
				selectedEnvPath = p
				break
			}
		}
	}
	cfg.EnvFilePath = selectedEnvPath

	// 3. Read existing .env if present
	envMap := make(map[string]string)
	if data, err := os.ReadFile(selectedEnvPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"'`)
				envMap[k] = v
				if os.Getenv(k) == "" {
					_ = os.Setenv(k, v)
				}
			}
		}
	}

	getVal := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		if v, ok := envMap[key]; ok && v != "" {
			return v
		}
		return fallback
	}

	// 4. Load or Auto-Generate Security Keys
	var generatedAny bool

	apiKey := getVal("API_KEY", "")
	if apiKey == "" {
		apiKey = "lms_" + GenerateRandomKey(16)
		generatedAny = true
	}

	apiSecret := getVal("API_SECRET", "")
	if apiSecret == "" {
		apiSecret = "sec_" + GenerateRandomKey(24)
		generatedAny = true
	}

	jwtSecret := getVal("JWT_SECRET", "")
	if jwtSecret == "" {
		jwtSecret = GenerateRandomKey(32)
		generatedAny = true
	}

	turnSecret := getVal("TURN_SECRET", "")
	if turnSecret == "" {
		turnSecret = "turn_" + GenerateRandomKey(20)
		generatedAny = true
	}

	_ = os.Setenv("API_KEY", apiKey)
	_ = os.Setenv("API_SECRET", apiSecret)
	_ = os.Setenv("JWT_SECRET", jwtSecret)
	_ = os.Setenv("TURN_SECRET", turnSecret)

	cfg.APIKey = apiKey
	cfg.APISecret = apiSecret
	cfg.JWTSecret = jwtSecret
	cfg.TurnSecret = turnSecret

	// 5. Port from YAML or env
	portStr := fmt.Sprintf("%d", cfg.YAML.Server.Port)
	if portStr == "0" {
		portStr = "8080"
	}
	cfg.Port = getVal("PORT", portStr)

	cfg.DomainName = getVal("DOMAIN_NAME", "")
	if cfg.DomainName == "" {
		cfg.DomainName = getVal("HOST_DOMAIN", "")
	}
	cfg.HostDomain = cfg.DomainName

	publicIP := getVal("PUBLIC_IP", "")
	if publicIP == "" && cfg.DomainName == "" {
		detected := DetectPublicIP()
		if detected != "" {
			publicIP = detected
			log.Printf("[Zero-Config] Auto-detected Server Public IP: %s\n", publicIP)
		} else {
			publicIP = "127.0.0.1"
		}
	} else if publicIP == "" && cfg.DomainName != "" {
		publicIP = cfg.DomainName
	}
	_ = os.Setenv("PUBLIC_IP", publicIP)
	cfg.PublicIP = publicIP

	turnRealm := getVal("TURN_REALM", "")
	if turnRealm == "" {
		if cfg.DomainName != "" {
			turnRealm = cfg.DomainName
		} else {
			turnRealm = fmt.Sprintf("turn.%s", cfg.PublicIP)
		}
	}
	_ = os.Setenv("TURN_REALM", turnRealm)
	cfg.TurnRealm = turnRealm

	cfg.RedisAddr = getVal("REDIS_ADDR", "")
	cfg.RedisPass = getVal("REDIS_PASSWORD", "")
	cfg.ServerRole = getVal("SERVER_ROLE", "origin")
	cfg.ServerID = getVal("SERVER_ID", "server-node-1")
	cfg.WebhookURL = getVal("WEBHOOK_TARGET_URL", "")

	// 6. Save auto-generated secrets to .env
	if generatedAny || len(envMap) == 0 {
		_ = os.MkdirAll(filepath.Dir(selectedEnvPath), 0755)
		content := fmt.Sprintf(`# OmniCast Engine - Auto-Generated Zero-Config Environment
PORT=%s
DOMAIN_NAME=%s
PUBLIC_IP=%s
HOST_DOMAIN=%s
TURN_REALM=%s
API_KEY=%s
API_SECRET=%s
JWT_SECRET=%s
TURN_SECRET=%s
REDIS_ADDR=%s
REDIS_PASSWORD=%s
SERVER_ROLE=%s
SERVER_ID=%s
WEBHOOK_TARGET_URL=%s
`, cfg.Port, cfg.DomainName, cfg.PublicIP, cfg.HostDomain, cfg.TurnRealm, cfg.APIKey, cfg.APISecret, cfg.JWTSecret, cfg.TurnSecret, cfg.RedisAddr, cfg.RedisPass, cfg.ServerRole, cfg.ServerID, cfg.WebhookURL)

		if err := os.WriteFile(selectedEnvPath, []byte(content), 0600); err == nil {
			log.Printf("[Zero-Config] Persisted auto-generated credentials to %s\n", selectedEnvPath)
		}
	}

	globalConfig = cfg
	return cfg
}

// PrintStartupBanner logs a styled, high-visibility banner with dynamic SSL / domain connection strings
func PrintStartupBanner(cfg *Config) {
	viewersCap := "UNLIMITED"
	if cfg.YAML.RoomManagement.MaxViewersPerRoom > 0 {
		viewersCap = fmt.Sprintf("%d", cfg.YAML.RoomManagement.MaxViewersPerRoom)
	}

	seatsCap := "UNLIMITED"
	if cfg.YAML.CoHosting.MaxActiveSeats > 0 {
		seatsCap = fmt.Sprintf("%d", cfg.YAML.CoHosting.MaxActiveSeats)
	}

	var banner string
	if cfg.DomainName != "" {
		// Custom Domain SSL Enabled Display (Auto-SSL via Caddy)
		banner = fmt.Sprintf(`
╔══════════════════════════════════════════════════════════════════════════╗
║                                                                          ║
║   🚀  OMNICAST ENGINE IS ONLINE & READY! (Auto-SSL Enabled)              ║
║                                                                          ║
╠══════════════════════════════════════════════════════════════════════════╣
║                                                                          ║
║   🌐  HOST URL     : wss://%s/ws
║   🌍  API URL      : https://%s/api
║   💻  WEB PORTAL   : https://%s
║   🔑  API KEY      : %s
║   🔒  API SECRET   : %s
║   🛡️  JWT SECRET   : %s
║   📡  COTURN REALM : %s
║   👥  VIEWERS CAP  : %s
║   🪑  CO-HOST CAP  : %s
║                                                                          ║
╠══════════════════════════════════════════════════════════════════════════╣
║  👉 Copy API_KEY & API_SECRET into your OmniCast Flutter / Mobile SDK!   ║
╚══════════════════════════════════════════════════════════════════════════╝
`, cfg.DomainName, cfg.DomainName, cfg.DomainName, cfg.APIKey, cfg.APISecret, cfg.JWTSecret, cfg.TurnRealm, viewersCap, seatsCap)
	} else {
		// Local / Direct IP Display
		hostDisplay := cfg.PublicIP
		banner = fmt.Sprintf(`
╔══════════════════════════════════════════════════════════════════════════╗
║                                                                          ║
║   🚀  OMNICAST ENGINE IS ONLINE & READY! (Zero-Config Plug & Play)       ║
║                                                                          ║
╠══════════════════════════════════════════════════════════════════════════╣
║                                                                          ║
║   🌐  HOST URL     : http://%s:%s
║   ⚡  WEBSOCKET    : ws://%s:%s/ws
║   🔑  API KEY      : %s
║   🔒  API SECRET   : %s
║   🛡️  JWT SECRET   : %s
║   📡  COTURN REALM : %s
║   👥  VIEWERS CAP  : %s
║   🪑  CO-HOST CAP  : %s
║                                                                          ║
╠══════════════════════════════════════════════════════════════════════════╣
║  👉 Copy API_KEY & API_SECRET into your OmniCast Flutter / Mobile SDK!   ║
╚══════════════════════════════════════════════════════════════════════════╝
`, hostDisplay, cfg.Port, hostDisplay, cfg.Port, cfg.APIKey, cfg.APISecret, cfg.JWTSecret, cfg.TurnRealm, viewersCap, seatsCap)
	}

	fmt.Println(banner)
}
