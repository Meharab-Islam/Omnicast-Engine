package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"omnicast/internal/models"
)

// DistributedParticipant represents participant metadata stored in Redis Hash
type DistributedParticipant struct {
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
	Role         string    `json:"role"` // "host", "cohost", "viewer"
	NodeIP       string    `json:"node_ip"`
	NodeID       string    `json:"node_id"`
	CanPublish   bool      `json:"can_publish"`
	CanSubscribe bool      `json:"can_subscribe"`
	JoinedAt     time.Time `json:"joined_at"`
}

// DistributedRoomMeta represents room metadata stored in Redis Hash (room:<room_id>:meta)
type DistributedRoomMeta struct {
	RoomID      string    `json:"room_id"`
	RoomName    string    `json:"room_name"`
	RoomType    string    `json:"room_type"` // "video", "audio"
	HostID      string    `json:"host_id"`
	HostNodeIP  string    `json:"host_node_ip"`
	HostNodeID  string    `json:"host_node_id"`
	Status      string    `json:"status"` // "active", "closed"
	ViewerCount int       `json:"viewer_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DistributedRoomManager manages cross-node room state and signaling bridges using Redis
type DistributedRoomManager struct {
	rdb       *redis.Client
	nodeID    string
	nodeIP    string
	jwtSecret string
	ctx       context.Context
}

// NewDistributedRoomManager creates a new DistributedRoomManager instance
func NewDistributedRoomManager(rdb *redis.Client, nodeID, nodeIP, jwtSecret string) *DistributedRoomManager {
	if jwtSecret == "" {
		jwtSecret = "live_media_server_jwt_secret_key_2026"
	}
	return &DistributedRoomManager{
		rdb:       rdb,
		nodeID:    nodeID,
		nodeIP:    nodeIP,
		jwtSecret: jwtSecret,
		ctx:       context.Background(),
	}
}

// 1. 🗄️ SaveRoomMeta: Stores active room state in Redis Hash (room:<room_id>:meta)
func (dm *DistributedRoomManager) SaveRoomMeta(ctx context.Context, meta DistributedRoomMeta) error {
	if dm.rdb == nil {
		return errors.New("redis client not initialized")
	}

	key := fmt.Sprintf("room:%s:meta", meta.RoomID)
	data := map[string]interface{}{
		"room_id":      meta.RoomID,
		"room_name":    meta.RoomName,
		"room_type":    meta.RoomType,
		"host_id":      meta.HostID,
		"host_node_ip": meta.HostNodeIP,
		"host_node_id": meta.HostNodeID,
		"status":       meta.Status,
		"viewer_count": meta.ViewerCount,
		"created_at":   meta.CreatedAt.Format(time.RFC3339),
		"updated_at":   time.Now().Format(time.RFC3339),
	}

	pipe := dm.rdb.Pipeline()
	pipe.HSet(ctx, key, data)
	pipe.Expire(ctx, key, 24*time.Hour)
	pipe.SAdd(ctx, "global:active_rooms", meta.RoomID)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save room meta to redis: %w", err)
	}

	log.Printf("[Distributed Room] Saved Room '%s' on Node %s (%s) to Redis Hash\n", meta.RoomID, meta.HostNodeID, meta.HostNodeIP)
	return nil
}

// GetRoomMeta retrieves room metadata from Redis Hash
func (dm *DistributedRoomManager) GetRoomMeta(ctx context.Context, roomID string) (*DistributedRoomMeta, error) {
	if dm.rdb == nil {
		return nil, errors.New("redis client not initialized")
	}

	key := fmt.Sprintf("room:%s:meta", roomID)
	val, err := dm.rdb.HGetAll(ctx, key).Result()
	if err != nil || len(val) == 0 {
		return nil, errors.New("room not found in redis")
	}

	meta := &DistributedRoomMeta{
		RoomID:     val["room_id"],
		RoomName:   val["room_name"],
		RoomType:   val["room_type"],
		HostID:     val["host_id"],
		HostNodeIP: val["host_node_ip"],
		HostNodeID: val["host_node_id"],
		Status:     val["status"],
	}
	return meta, nil
}

// 2. 👥 AddParticipant: Stores participant in Redis Hash (room:<room_id>:participants)
func (dm *DistributedRoomManager) AddParticipant(ctx context.Context, roomID string, p DistributedParticipant) error {
	if dm.rdb == nil {
		return errors.New("redis client not initialized")
	}

	pKey := fmt.Sprintf("room:%s:participants", roomID)
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}

	pipe := dm.rdb.Pipeline()
	pipe.HSet(ctx, pKey, p.UserID, data)
	pipe.Expire(ctx, pKey, 24*time.Hour)
	if p.Role == "viewer" {
		mKey := fmt.Sprintf("room:%s:meta", roomID)
		pipe.HIncrBy(ctx, mKey, "viewer_count", 1)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// RemoveParticipant removes a participant from the Redis Hash
func (dm *DistributedRoomManager) RemoveParticipant(ctx context.Context, roomID, userID string) error {
	if dm.rdb == nil {
		return errors.New("redis client not initialized")
	}

	pKey := fmt.Sprintf("room:%s:participants", roomID)
	pipe := dm.rdb.Pipeline()
	pipe.HDel(ctx, pKey, userID)
	mKey := fmt.Sprintf("room:%s:meta", roomID)
	pipe.HIncrBy(ctx, mKey, "viewer_count", -1)
	_, err := pipe.Exec(ctx)
	return err
}

// 3. 🌉 BridgeSignaling: Cross-Node Signaling Sync (Offer/Answer/ICE) via Redis Pub/Sub
func (dm *DistributedRoomManager) BridgeSignaling(ctx context.Context, targetRoomID, targetUserID string, msg *models.SignalingMessage) error {
	if dm.rdb == nil {
		return errors.New("redis client not initialized")
	}

	channel := fmt.Sprintf("signaling.%s.%s", targetRoomID, targetUserID)
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return dm.rdb.Publish(ctx, channel, data).Err()
}

// 4. 🔒 AuthenticateTokenClaims verifies JWT token and extracts granular permissions
type OmnicastAuthClaims struct {
	UserID       string `json:"user_id"`
	RoomID       string `json:"room_id"`
	Role         string `json:"role"` // "host", "cohost", "viewer"
	CanPublish   bool   `json:"can_publish"`
	CanSubscribe bool   `json:"can_subscribe"`
	jwt.RegisteredClaims
}

// VerifyWebSocketToken parses and validates a signed JWT token for WebSocket authentication
func (dm *DistributedRoomManager) VerifyWebSocketToken(tokenString string) (*OmnicastAuthClaims, error) {
	if tokenString == "" {
		return nil, errors.New("missing authentication token")
	}

	token, err := jwt.ParseWithClaims(tokenString, &OmnicastAuthClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(dm.jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token signature or format: %w", err)
	}

	claims, ok := token.Claims.(*OmnicastAuthClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// Validate required fields
	if claims.UserID == "" {
		return nil, errors.New("token missing user_id")
	}

	// Automatically assign permissions based on role if not set
	if claims.Role == "host" || claims.Role == "broadcaster" {
		claims.CanPublish = true
		claims.CanSubscribe = true
	} else if claims.Role == "cohost" {
		claims.CanPublish = true
		claims.CanSubscribe = true
	} else if claims.Role == "viewer" || claims.Role == "" {
		claims.Role = "viewer"
		claims.CanSubscribe = true
	}

	return claims, nil
}

// CheckPermission verifies if the claims permit a specific action (e.g. "publish" or "subscribe")
func (dm *DistributedRoomManager) CheckPermission(claims *OmnicastAuthClaims, action string) error {
	if claims == nil {
		return errors.New("unauthorized: missing claims")
	}

	switch strings.ToLower(action) {
	case "publish", "offer_broadcast":
		if !claims.CanPublish {
			return fmt.Errorf("forbidden: user '%s' does not have can_publish permission", claims.UserID)
		}
	case "subscribe", "offer_view":
		if !claims.CanSubscribe {
			return fmt.Errorf("forbidden: user '%s' does not have can_subscribe permission", claims.UserID)
		}
	}
	return nil
}
