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
	"time"

	"github.com/pion/ice/v2"
	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v3"
)

// NetworkConfig holds the network setup parameters for Omnicast SFU
type NetworkConfig struct {
	Port         int    // Single Port for UDP/TCP WebRTC and TURN (e.g. 3478)
	PublicIP     string // Public IP advertised in NAT 1:1 candidate mapping
	Realm        string // Realm for STUN/TURN (e.g. "omnicast.live")
	AuthSecret   string // Shared secret for HMAC-SHA1 short-lived tokens
	RelayMinPort uint16 // Minimum port for allocated TURN relay sockets (e.g. 50000)
	RelayMaxPort uint16 // Maximum port for allocated TURN relay sockets (e.g. 50200)
}

// OmnicastNetworkStack manages the single-port multiplexers and embedded TURN server
type OmnicastNetworkStack struct {
	SettingEngine webrtc.SettingEngine
	UDPMux        ice.UDPMux
	TCPMux        ice.TCPMux
	TURNServer    *turn.Server
	UDPConn       net.PacketConn
	TCPListener   net.Listener
	Config        NetworkConfig
}

// InitOmnicastNetworkLayer initializes single-port WebRTC multiplexing (UDPMux & TCPMux)
// and an embedded Pion STUN/TURN server running concurrently on port 3478
func InitOmnicastNetworkLayer(cfg NetworkConfig) (*OmnicastNetworkStack, error) {
	if cfg.Port <= 0 {
		cfg.Port = 3478
	}
	if cfg.Realm == "" {
		cfg.Realm = "omnicast.live"
	}
	if cfg.AuthSecret == "" {
		cfg.AuthSecret = DefaultTURNSecret
	}
	if cfg.PublicIP == "" {
		cfg.PublicIP = "178.162.252.30"
	}
	if cfg.RelayMinPort == 0 {
		cfg.RelayMinPort = 50000
	}
	if cfg.RelayMaxPort == 0 {
		cfg.RelayMaxPort = 50200
	}

	stack := &OmnicastNetworkStack{
		SettingEngine: webrtc.SettingEngine{},
		Config:        cfg,
	}

	// 1. 🔀 Port Multiplexing (pion/ice UDPMux): Bind single UDP socket
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: cfg.Port})
	if err != nil {
		return nil, fmt.Errorf("failed to bind UDP port %d for UDPMux: %w", cfg.Port, err)
	}
	stack.UDPConn = udpConn

	udpMux := webrtc.NewICEUDPMux(nil, udpConn)
	stack.UDPMux = udpMux
	stack.SettingEngine.SetICEUDPMux(udpMux)

	// 2. 🔀 Port Multiplexing (pion/ice TCPMux): Bind single TCP listener (Passive ICE TCP)
	tcpListener, tcpErr := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if tcpErr == nil {
		stack.TCPListener = tcpListener
		tcpMux := webrtc.NewICETCPMux(nil, tcpListener, 20)
		stack.TCPMux = tcpMux
		stack.SettingEngine.SetICETCPMux(tcpMux)
		log.Printf("[Omnicast Network] TCPMux active on 0.0.0.0:%d (Firewall Bypass Enabled)\n", cfg.Port)
	} else {
		log.Printf("[Omnicast Network] TCP port %d notice: %v (operating over UDP)\n", cfg.Port, tcpErr)
	}

	// 3. 🛡️ Embedded STUN/TURN Server (pion/turn): Token-Based REST API Auth
	relayAddressGen := &turn.RelayAddressGeneratorPortRange{
		RelayAddress: net.ParseIP(cfg.PublicIP),
		Address:      "0.0.0.0",
		MinPort:      cfg.RelayMinPort,
		MaxPort:      cfg.RelayMaxPort,
	}

	turnServer, turnErr := turn.NewServer(turn.ServerConfig{
		Realm: cfg.Realm,
		// Dynamic HMAC Token AuthHandler (RFC 5389 / TURN REST API)
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			return ValidateHMACToken(username, realm, cfg.AuthSecret)
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn:            udpConn,
				RelayAddressGenerator: relayAddressGen,
			},
		},
	})
	if turnErr != nil {
		log.Printf("[Omnicast Network] Embedded TURN notice: %v\n", turnErr)
	} else {
		stack.TURNServer = turnServer
		log.Printf("[Omnicast Network] Embedded STUN/TURN server running concurrently on UDP :%d (Realm: %s)\n", cfg.Port, cfg.Realm)
	}

	// 4. NAT 1:1 Public IP Mapping & MTU Buffer Optimization
	stack.SettingEngine.SetNAT1To1IPs([]string{cfg.PublicIP}, webrtc.ICECandidateTypeHost)
	stack.SettingEngine.SetReceiveMTU(1500)
	stack.SettingEngine.SetICETimeouts(15*time.Second, 30*time.Second, 2*time.Second)

	log.Printf("[Omnicast Network] Network layer initialized: Single Port :%d multiplexing active\n", cfg.Port)
	return stack, nil
}

// ValidateHMACToken validates short-lived time-based tokens and computes the dynamic MD5 key
func ValidateHMACToken(username, realm, secret string) ([]byte, bool) {
	if username == "" || secret == "" {
		return nil, false
	}

	// Token format: <unix_timestamp_expiry>:<user_id>
	parts := strings.SplitN(username, ":", 2)
	if len(parts) != 2 {
		return nil, false // Missing expiry timestamp
	}

	expiryUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, false // Invalid timestamp
	}

	// Reject expired tokens
	if time.Now().Unix() > expiryUnix {
		log.Printf("[TURN Auth] Expired token rejected for user '%s'\n", username)
		return nil, false
	}

	// Compute expected HMAC-SHA1 password
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	expectedPassword := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return turn.GenerateAuthKey(username, realm, expectedPassword), true
}

// Close gracefully closes all network listeners and stops the TURN server
func (s *OmnicastNetworkStack) Close() error {
	if s.TURNServer != nil {
		_ = s.TURNServer.Close()
	}
	if s.UDPConn != nil {
		_ = s.UDPConn.Close()
	}
	if s.TCPListener != nil {
		_ = s.TCPListener.Close()
	}
	return nil
}
