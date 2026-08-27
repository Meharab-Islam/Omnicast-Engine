package webrtc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/pion/webrtc/v3"
)

// Default fallback configuration values
const (
	DefaultTURNSecret = "my_super_secure_turn_secret_999"
	DefaultTURNRealm  = "live.myvps.com"
	DefaultTURNTTL    = 24 * time.Hour
)

// ICEServerJSON represents a JSON-serializable ICE Server definition for SDK clients
type ICEServerJSON struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// GenerateTURNCredentials generates standard time-limited HMAC-SHA1 TURN credentials (TURN REST API)
func GenerateTURNCredentials(userID, secret string, duration time.Duration) (string, string) {
	if secret == "" {
		secret = os.Getenv("TURN_SECRET")
		if secret == "" {
			secret = DefaultTURNSecret
		}
	}
	if userID == "" {
		userID = "anonymous"
	}
	if duration <= 0 {
		duration = DefaultTURNTTL
	}

	expiry := time.Now().Add(duration).Unix()
	username := fmt.Sprintf("%d:%s", expiry, userID)

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return username, password
}

// GetDefaultICEServers constructs Pion webrtc.ICEServer slice configured with dynamic TURN REST credentials
func GetDefaultICEServers(userID string) []webrtc.ICEServer {
	publicIP := os.Getenv("PUBLIC_IP")
	if publicIP == "" {
		publicIP = "192.168.0.116"
	}
	turnSecret := os.Getenv("TURN_SECRET")
	if turnSecret == "" {
		turnSecret = DefaultTURNSecret
	}

	username, password := GenerateTURNCredentials(userID, turnSecret, 24*time.Hour)

	return []webrtc.ICEServer{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				fmt.Sprintf("stun:%s:3478", publicIP),
			},
		},
		{
			URLs: []string{
				fmt.Sprintf("turn:%s:3478?transport=udp", publicIP),
				fmt.Sprintf("turn:%s:3478?transport=tcp", publicIP),
			},
			Username:       username,
			Credential:     password,
			CredentialType: webrtc.ICECredentialTypePassword,
		},
	}
}

// GetDefaultICEServersJSON returns JSON-serializable ICE servers array to pass directly to frontend SDKs
func GetDefaultICEServersJSON(userID string) []ICEServerJSON {
	publicIP := os.Getenv("PUBLIC_IP")
	if publicIP == "" {
		publicIP = "192.168.0.116"
	}
	turnSecret := os.Getenv("TURN_SECRET")
	if turnSecret == "" {
		turnSecret = DefaultTURNSecret
	}

	username, password := GenerateTURNCredentials(userID, turnSecret, 24*time.Hour)

	return []ICEServerJSON{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				fmt.Sprintf("stun:%s:3478", publicIP),
			},
		},
		{
			URLs: []string{
				fmt.Sprintf("turn:%s:3478?transport=udp", publicIP),
				fmt.Sprintf("turn:%s:3478?transport=tcp", publicIP),
			},
			Username:   username,
			Credential: password,
		},
	}
}

// GetDynamicRTCConfiguration returns a webrtc.Configuration pre-populated with dynamic TURN credentials
func GetDynamicRTCConfiguration(userID string) webrtc.Configuration {
	return webrtc.Configuration{
		ICEServers: GetDefaultICEServers(userID),
	}
}
