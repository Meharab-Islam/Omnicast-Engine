package webrtc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/turn/v2"
)

// EmbeddedTURNConfig holds configuration for the embedded Pion TURN server
type EmbeddedTURNConfig struct {
	PublicIP   string
	Port       int
	TCPPort    int
	Realm      string
	AuthSecret string
}

// EmbeddedTURNServer wraps Pion's TURN server v2 for zero-dependency embedded NAT traversal
type EmbeddedTURNServer struct {
	server *turn.Server
	config EmbeddedTURNConfig
	mu     sync.Mutex
	closed bool
}

// GenerateAuthKeyWithSecret generates the standard MD5(username:realm:password) auth key
// where password is the base64-encoded HMAC-SHA1 of the username using the shared secret.
func GenerateAuthKeyWithSecret(username, realm, secret string) []byte {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return turn.GenerateAuthKey(username, realm, password)
}

// ValidateAndGenerateAuthKey validates that temporary credentials have not expired and returns the key
func ValidateAndGenerateAuthKey(username, realm, secret string) ([]byte, bool) {
	// Parse timestamp from format: "<unix_timestamp>:<user_id>"
	parts := strings.SplitN(username, ":", 2)
	if len(parts) == 2 {
		expiry, err := strconv.ParseInt(parts[0], 10, 64)
		if err == nil {
			if time.Now().Unix() > expiry {
				log.Printf("[TURN Auth] Expired temporary credential rejected for user: %s (expired at: %d)\n", username, expiry)
				return nil, false // Expired credential rejected
			}
		}
	}

	key := GenerateAuthKeyWithSecret(username, realm, secret)
	return key, true
}

// NewEmbeddedTURNServer initializes and starts an embedded Pion TURN server with time-limited AuthHandler
func NewEmbeddedTURNServer(cfg EmbeddedTURNConfig) (*EmbeddedTURNServer, error) {
	if cfg.Port <= 0 {
		cfg.Port = 3478
	}
	if cfg.Realm == "" {
		cfg.Realm = "omnicast.live"
	}
	if cfg.PublicIP == "" {
		cfg.PublicIP = "127.0.0.1"
	}
	if cfg.AuthSecret == "" {
		cfg.AuthSecret = "omnicast_secret_turn_key"
	}

	udpListener, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to bind embedded TURN UDP port %d: %w", cfg.Port, err)
	}

	relayGen := &turn.RelayAddressGeneratorPortRange{
		RelayAddress: net.ParseIP(cfg.PublicIP),
		Address:      "0.0.0.0",
		MinPort:      50000,
		MaxPort:      50200,
	}

	listenerConfigs := []turn.ListenerConfig{}
	if cfg.TCPPort > 0 {
		tcpListener, tcpErr := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", cfg.TCPPort))
		if tcpErr == nil {
			listenerConfigs = append(listenerConfigs, turn.ListenerConfig{
				Listener:              tcpListener,
				RelayAddressGenerator: relayGen,
			})
			log.Printf("[Embedded TURN] TCP listener active on 0.0.0.0:%d for strict firewall bypass\n", cfg.TCPPort)
		} else {
			log.Printf("[Embedded TURN] TCP port %d status: %v\n", cfg.TCPPort, tcpErr)
		}
	}

	serverConfig := turn.ServerConfig{
		Realm: cfg.Realm,
		// Dynamic REST Auth Handler: Validates temporary time-limited credentials using shared secret
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			return ValidateAndGenerateAuthKey(username, realm, cfg.AuthSecret)
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn:            udpListener,
				RelayAddressGenerator: relayGen,
			},
		},
		ListenerConfigs: listenerConfigs,
	}

	server, err := turn.NewServer(serverConfig)
	if err != nil {
		_ = udpListener.Close()
		return nil, fmt.Errorf("failed to create embedded TURN server: %w", err)
	}

	log.Printf("[Embedded TURN] Pion TURN v2 server running on UDP 0.0.0.0:%d (Public IP: %s, Realm: %s)\n",
		cfg.Port, cfg.PublicIP, cfg.Realm)

	return &EmbeddedTURNServer{
		server: server,
		config: cfg,
	}, nil
}

// GenerateEphemeralCredentials creates time-limited TURN REST credentials for WebRTC clients
func (s *EmbeddedTURNServer) GenerateEphemeralCredentials(username string, ttl time.Duration) (string, string) {
	if s == nil {
		return "", ""
	}

	expiry := time.Now().Add(ttl).Unix()
	turnUsername := fmt.Sprintf("%d:%s", expiry, username)

	mac := hmac.New(sha1.New, []byte(s.config.AuthSecret))
	mac.Write([]byte(turnUsername))
	turnPassword := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return turnUsername, turnPassword
}

// Close gracefully stops the embedded TURN server and releases allocated relay ports
func (s *EmbeddedTURNServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.server != nil {
		log.Println("[Embedded TURN] Shutting down embedded TURN server...")
		return s.server.Close()
	}
	return nil
}

// ValidateEphemeralCredential checks if a username's timestamp has not expired
func ValidateEphemeralCredential(username string) bool {
	var expiryStr string
	for i, c := range username {
		if c == ':' {
			expiryStr = username[:i]
			break
		}
	}
	if expiryStr == "" {
		return true
	}
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() <= expiry
}
