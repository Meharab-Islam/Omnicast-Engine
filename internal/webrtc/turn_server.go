package webrtc

import (
	"fmt"
	"log"
	"net"

	"github.com/pion/turn/v2"
)

// EmbeddedTURNServerConfig defines settings for the in-process Pion STUN/TURN server
type EmbeddedTURNServerConfig struct {
	PublicIP string
	Realm    string
	Secret   string
	Port     int
	MinPort  uint16
	MaxPort  uint16
}

// StartEmbeddedTURNServer initializes and runs an embedded STUN/TURN server on UDP concurrently
func StartEmbeddedTURNServer(cfg EmbeddedTURNServerConfig) (*turn.Server, error) {
	if cfg.Port <= 0 {
		cfg.Port = 3478
	}
	if cfg.Realm == "" {
		cfg.Realm = "omnicast.live"
	}
	if cfg.Secret == "" {
		cfg.Secret = DefaultTURNSecret
	}
	if cfg.PublicIP == "" {
		cfg.PublicIP = "127.0.0.1"
	}
	if cfg.MinPort == 0 {
		cfg.MinPort = 50000
	}
	if cfg.MaxPort == 0 {
		cfg.MaxPort = 50200
	}

	udpListener, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to bind UDP port %d: %w", cfg.Port, err)
	}

	relayGen := &turn.RelayAddressGeneratorPortRange{
		RelayAddress: net.ParseIP(cfg.PublicIP),
		Address:      "0.0.0.0",
		MinPort:      cfg.MinPort,
		MaxPort:      cfg.MaxPort,
	}

	server, err := turn.NewServer(turn.ServerConfig{
		Realm: cfg.Realm,
		// Dynamic Token Auth Handler (RFC 5389 / TURN REST API HMAC-SHA1)
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			return ValidateAndGenerateAuthKey(username, realm, cfg.Secret)
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn:            udpListener,
				RelayAddressGenerator: relayGen,
			},
		},
	})
	if err != nil {
		_ = udpListener.Close()
		return nil, fmt.Errorf("failed to create turn server: %w", err)
	}

	log.Printf("[Embedded TURN] STUN/TURN server running concurrently on UDP 0.0.0.0:%d (Public IP: %s, Realm: %s, Relay Ports: %d-%d)\n",
		cfg.Port, cfg.PublicIP, cfg.Realm, cfg.MinPort, cfg.MaxPort)

	return server, nil
}
