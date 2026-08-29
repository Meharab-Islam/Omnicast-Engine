package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"omnicast/internal/models"
)

// MessageHandler is the callback function invoked when a Redis message is received
type MessageHandler func(roomID string, msg *models.SignalingMessage)

// RedisBroker manages Redis client connection, publishing, and distributed Pub/Sub subscriptions
type RedisBroker struct {
	client      *redis.Client
	ctx         context.Context
	cancel      context.CancelFunc
	handler     MessageHandler
	mu          sync.RWMutex
	isActive    bool
	subscribers map[string]*redis.PubSub
}

// NewRedisBroker creates and initializes a new RedisBroker instance
func NewRedisBroker(addr, password string, db int) (*RedisBroker, error) {
	if addr == "" {
		return nil, errors.New("redis address is empty")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Test connection with Ping
	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pingCancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancel()
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping failed (%s): %w", addr, err)
	}

	broker := &RedisBroker{
		client:      rdb,
		ctx:         ctx,
		cancel:      cancel,
		isActive:    true,
		subscribers: make(map[string]*redis.PubSub),
	}

	log.Printf("[Redis] Successfully connected to Redis at %s\n", addr)
	return broker, nil
}

// SetMessageHandler registers the callback function for processing incoming Pub/Sub messages
func (rb *RedisBroker) SetMessageHandler(handler MessageHandler) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.handler = handler
}

// IsActive returns whether the Redis broker is actively connected
func (rb *RedisBroker) IsActive() bool {
	if rb == nil {
		return false
	}
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.isActive
}

// FormatViewerSignalingChannel returns the standardized Redis Pub/Sub channel format:
// signaling.<room_id>.<viewer_id>
func FormatViewerSignalingChannel(roomID, viewerID string) string {
	return fmt.Sprintf("signaling.%s.%s", roomID, viewerID)
}

// PublishViewerSignaling publishes a direct signaling message to the dedicated channel signaling.<room_id>.<viewer_id>
func (rb *RedisBroker) PublishViewerSignaling(roomID, viewerID string, msg *models.SignalingMessage) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	channel := FormatViewerSignalingChannel(roomID, viewerID)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal viewer signaling message: %w", err)
	}

	ctx, cancel := context.WithTimeout(rb.ctx, 2*time.Second)
	defer cancel()

	if err := rb.client.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("failed to publish to viewer channel %s: %w", channel, err)
	}

	return nil
}

// SubscribeViewerSignaling subscribes to the dedicated channel signaling.<room_id>.<viewer_id>
// and invokes the onMessage callback whenever an inter-node signaling message arrives for this viewer.
func (rb *RedisBroker) SubscribeViewerSignaling(roomID, viewerID string, onMessage func(msg *models.SignalingMessage)) (*redis.PubSub, error) {
	if rb == nil || !rb.IsActive() {
		return nil, errors.New("redis broker is not active")
	}

	channel := FormatViewerSignalingChannel(roomID, viewerID)
	rb.mu.Lock()
	if sub, exists := rb.subscribers[channel]; exists && sub != nil {
		rb.mu.Unlock()
		return sub, nil
	}

	pubsub := rb.client.Subscribe(rb.ctx, channel)
	rb.subscribers[channel] = pubsub
	rb.mu.Unlock()

	go func() {
		ch := pubsub.Channel()
		for {
			select {
			case <-rb.ctx.Done():
				return
			case redisMsg, ok := <-ch:
				if !ok {
					return
				}
				var sigMsg models.SignalingMessage
				if err := json.Unmarshal([]byte(redisMsg.Payload), &sigMsg); err == nil && onMessage != nil {
					onMessage(&sigMsg)
				}
			}
		}
	}()

	log.Printf("[Redis] Subscribed to viewer signaling channel %s\n", channel)
	return pubsub, nil
}

// UnsubscribeViewerSignaling unsubscribes and closes the channel signaling.<room_id>.<viewer_id>
func (rb *RedisBroker) UnsubscribeViewerSignaling(roomID, viewerID string) {
	if rb == nil {
		return
	}

	channel := FormatViewerSignalingChannel(roomID, viewerID)
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if pubsub, exists := rb.subscribers[channel]; exists && pubsub != nil {
		_ = pubsub.Close()
		delete(rb.subscribers, channel)
		log.Printf("[Redis] Unsubscribed from viewer signaling channel %s\n", channel)
	}
}

// SubscribeRoomSignalingPattern subscribes to the pattern signaling.<room_id>.* using Redis PSubscribe.
// Origin Node A listens to this pattern to process incoming remote viewer offers and ICE candidates from Edge nodes.
func (rb *RedisBroker) SubscribeRoomSignalingPattern(roomID string, onMessage func(viewerID string, msg *models.SignalingMessage)) (*redis.PubSub, error) {
	if rb == nil || !rb.IsActive() {
		return nil, errors.New("redis broker is not active")
	}

	pattern := fmt.Sprintf("signaling.%s.*", roomID)
	rb.mu.Lock()
	if sub, exists := rb.subscribers[pattern]; exists && sub != nil {
		rb.mu.Unlock()
		return sub, nil
	}

	pubsub := rb.client.PSubscribe(rb.ctx, pattern)
	rb.subscribers[pattern] = pubsub
	rb.mu.Unlock()

	go func() {
		ch := pubsub.Channel()
		prefix := fmt.Sprintf("signaling.%s.", roomID)
		for {
			select {
			case <-rb.ctx.Done():
				return
			case redisMsg, ok := <-ch:
				if !ok {
					return
				}
				viewerID := strings.TrimPrefix(redisMsg.Channel, prefix)
				var sigMsg models.SignalingMessage
				if err := json.Unmarshal([]byte(redisMsg.Payload), &sigMsg); err == nil && onMessage != nil {
					onMessage(viewerID, &sigMsg)
				}
			}
		}
	}()

	log.Printf("[Redis Pattern] Node A subscribed to pattern %s\n", pattern)
	return pubsub, nil
}

// UnsubscribeRoomSignalingPattern stops listening to pattern signaling.<room_id>.*
func (rb *RedisBroker) UnsubscribeRoomSignalingPattern(roomID string) {
	if rb == nil {
		return
	}

	pattern := fmt.Sprintf("signaling.%s.*", roomID)
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if pubsub, exists := rb.subscribers[pattern]; exists && pubsub != nil {
		_ = pubsub.Close()
		delete(rb.subscribers, pattern)
		log.Printf("[Redis Pattern] Unsubscribed from pattern %s\n", pattern)
	}
}

// PublishRoomEvent publishes a signaling event (chat, gift, seat, pk, etc.) to the room's Redis channel
func (rb *RedisBroker) PublishRoomEvent(roomID string, msg *models.SignalingMessage) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	channel := fmt.Sprintf("room:%s:events", roomID)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message for redis: %w", err)
	}

	ctx, cancel := context.WithTimeout(rb.ctx, 2*time.Second)
	defer cancel()

	if err := rb.client.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("failed to publish to channel %s: %w", channel, err)
	}

	return nil
}

// SubscribeRoom starts listening for events published to a specific room channel
func (rb *RedisBroker) SubscribeRoom(roomID string) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	channel := fmt.Sprintf("room:%s:events", roomID)
	if _, exists := rb.subscribers[roomID]; exists {
		return nil // already subscribed
	}

	pubsub := rb.client.Subscribe(rb.ctx, channel)
	rb.subscribers[roomID] = pubsub

	// Start background listener for this channel
	go rb.listenChannel(roomID, pubsub)
	log.Printf("[Redis] Subscribed to channel %s\n", channel)
	return nil
}

// UnsubscribeRoom stops listening for events on a room channel
func (rb *RedisBroker) UnsubscribeRoom(roomID string) {
	if rb == nil {
		return
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	if pubsub, exists := rb.subscribers[roomID]; exists && pubsub != nil {
		_ = pubsub.Close()
		delete(rb.subscribers, roomID)
		log.Printf("[Redis] Unsubscribed from channel room:%s:events\n", roomID)
	}
}

// listenChannel listens to a Redis PubSub channel and dispatches messages to the registered handler
func (rb *RedisBroker) listenChannel(roomID string, pubsub *redis.PubSub) {
	ch := pubsub.Channel()
	for {
		select {
		case <-rb.ctx.Done():
			return
		case redisMsg, ok := <-ch:
			if !ok {
				return
			}
			var sigMsg models.SignalingMessage
			if err := json.Unmarshal([]byte(redisMsg.Payload), &sigMsg); err != nil {
				log.Printf("[Redis] Error decoding message from channel %s: %v\n", redisMsg.Channel, err)
				continue
			}

			rb.mu.RLock()
			handler := rb.handler
			rb.mu.RUnlock()

			if handler != nil {
				handler(roomID, &sigMsg)
			}
		}
	}
}

// RegisterRoomOrigin registers the room's origin server address in Redis with an optional TTL
func (rb *RedisBroker) RegisterRoomOrigin(roomID, originAddr string, ttl time.Duration) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	key := fmt.Sprintf("room:origin:%s", roomID)
	ctx, cancel := context.WithTimeout(rb.ctx, 2*time.Second)
	defer cancel()

	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	if err := rb.client.Set(ctx, key, originAddr, ttl).Err(); err != nil {
		return fmt.Errorf("failed to register room origin for %s: %w", roomID, err)
	}

	log.Printf("[Redis Registry] Registered Room '%s' -> Origin Server: %s\n", roomID, originAddr)
	return nil
}

// GetRoomOrigin retrieves the origin server address hosting the specified room
func (rb *RedisBroker) GetRoomOrigin(roomID string) (string, error) {
	if rb == nil || !rb.IsActive() {
		return "", errors.New("redis broker is not active")
	}

	key := fmt.Sprintf("room:origin:%s", roomID)
	ctx, cancel := context.WithTimeout(rb.ctx, 2*time.Second)
	defer cancel()

	originAddr, err := rb.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", errors.New("room origin not found in redis registry: " + roomID)
		}
		return "", fmt.Errorf("failed to query room origin for %s: %w", roomID, err)
	}

	return originAddr, nil
}

// RemoveRoomOrigin deletes the room's origin mapping from Redis
func (rb *RedisBroker) RemoveRoomOrigin(roomID string) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	key := fmt.Sprintf("room:origin:%s", roomID)
	ctx, cancel := context.WithTimeout(rb.ctx, 2*time.Second)
	defer cancel()

	if err := rb.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete room origin for %s: %w", roomID, err)
	}

	log.Printf("[Redis Registry] Removed Room '%s' origin registry\n", roomID)
	return nil
}

// SaveRoomState serializes and caches the RoomState in Redis with a 24h TTL
func (rb *RedisBroker) SaveRoomState(ctx context.Context, state *models.RoomState) error {
	if rb == nil || !rb.IsActive() || state == nil {
		return errors.New("redis broker is not active or state is nil")
	}

	key := fmt.Sprintf("room:%s:state", state.RoomID)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal room state: %w", err)
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	return rb.client.Set(ctx, key, data, 24*time.Hour).Err()
}

// GetRoomState retrieves and deserializes the cached RoomState from Redis
func (rb *RedisBroker) GetRoomState(ctx context.Context, roomID string) (*models.RoomState, error) {
	if rb == nil || !rb.IsActive() {
		return nil, errors.New("redis broker is not active")
	}

	key := fmt.Sprintf("room:%s:state", roomID)
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	data, err := rb.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errors.New("room state not found in redis: " + roomID)
		}
		return nil, fmt.Errorf("failed to get room state for %s: %w", roomID, err)
	}

	var state models.RoomState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room state: %w", err)
	}

	return &state, nil
}

// GetRoomStatesBatch retrieves multiple RoomStates concurrently using a Redis Pipeline
func (rb *RedisBroker) GetRoomStatesBatch(ctx context.Context, roomIDs []string) (map[string]*models.RoomState, error) {
	if rb == nil || !rb.IsActive() {
		return nil, errors.New("redis broker is not active")
	}
	if len(roomIDs) == 0 {
		return make(map[string]*models.RoomState), nil
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 3*time.Second)
		defer cancel()
	}

	pipe := rb.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd, len(roomIDs))
	for _, id := range roomIDs {
		key := fmt.Sprintf("room:%s:state", id)
		cmds[id] = pipe.Get(ctx, key)
	}

	_, _ = pipe.Exec(ctx)

	results := make(map[string]*models.RoomState, len(roomIDs))
	for id, cmd := range cmds {
		data, err := cmd.Bytes()
		if err == nil {
			var state models.RoomState
			if unmarshalErr := json.Unmarshal(data, &state); unmarshalErr == nil {
				results[id] = &state
			}
		}
	}

	return results, nil
}

// BatchIncrementScores atomically applies accumulated score deltas for multiple rooms using Redis Pipelining
func (rb *RedisBroker) BatchIncrementScores(ctx context.Context, scoreDeltas map[string]int64) error {
	if rb == nil || !rb.IsActive() || len(scoreDeltas) == 0 {
		return nil
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 3*time.Second)
		defer cancel()
	}

	pipe := rb.client.Pipeline()
	for roomID, delta := range scoreDeltas {
		if delta > 0 {
			key := fmt.Sprintf("room:%s:score", roomID)
			pipe.IncrBy(ctx, key, delta)
		}
	}

	_, err := pipe.Exec(ctx)
	return err
}

// IncrementHostScore atomically increments the gift score for a room using Redis INCRBY
func (rb *RedisBroker) IncrementHostScore(ctx context.Context, roomID string, coins int64) (int64, error) {
	if rb == nil || !rb.IsActive() {
		return 0, errors.New("redis broker is not active")
	}

	key := fmt.Sprintf("room:%s:score", roomID)
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	newScore, err := rb.client.IncrBy(ctx, key, coins).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment host score for %s: %w", roomID, err)
	}

	// Update cached RoomState if it exists in Redis
	if state, err := rb.GetRoomState(ctx, roomID); err == nil && state != nil {
		state.HostScore = newScore
		_ = rb.SaveRoomState(ctx, state)
	}

	return newScore, nil
}

// GetHostScore fetches the current host score from Redis
func (rb *RedisBroker) GetHostScore(ctx context.Context, roomID string) (int64, error) {
	if rb == nil || !rb.IsActive() {
		return 0, errors.New("redis broker is not active")
	}

	key := fmt.Sprintf("room:%s:score", roomID)
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	score, err := rb.client.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}

	return score, nil
}

// DeleteRoomState removes the cached RoomState and score from Redis
func (rb *RedisBroker) DeleteRoomState(ctx context.Context, roomID string) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	stateKey := fmt.Sprintf("room:%s:state", roomID)
	scoreKey := fmt.Sprintf("room:%s:score", roomID)

	return rb.client.Del(ctx, stateKey, scoreKey).Err()
}

// SetRoomNodeMap stores the key-value pair room_node_map:<room_id> = <current_node_id> in Redis with a TTL.
func (rb *RedisBroker) SetRoomNodeMap(ctx context.Context, roomID, nodeID string, ttl time.Duration) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	if ctx == nil {
		ctx = context.TODO()
	}

	key := fmt.Sprintf("room_node_map:%s", roomID)
	return rb.client.Set(ctx, key, nodeID, ttl).Err()
}

// GetRoomNodeMap retrieves the node_id hosting the room: room_node_map:<room_id>
func (rb *RedisBroker) GetRoomNodeMap(ctx context.Context, roomID string) (string, error) {
	if rb == nil || !rb.IsActive() {
		return "", errors.New("redis broker is not active")
	}

	if ctx == nil {
		ctx = context.TODO()
	}

	key := fmt.Sprintf("room_node_map:%s", roomID)
	val, err := rb.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// UnlinkAllRoomKeys instantly and non-blockingly wipes all Redis keys associated with a room using UNLINK
func (rb *RedisBroker) UnlinkAllRoomKeys(ctx context.Context, roomID string) error {
	if rb == nil || !rb.IsActive() {
		return nil
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	keys := []string{
		fmt.Sprintf("room:%s:state", roomID),
		fmt.Sprintf("room:%s:score", roomID),
		fmt.Sprintf("room:%s:chats", roomID),
		fmt.Sprintf("room:%s:participants", roomID),
		fmt.Sprintf("room:%s:banned", roomID),
		fmt.Sprintf("room:%s:origin", roomID),
		fmt.Sprintf("room_node_map:%s", roomID),
		fmt.Sprintf("pk:session:%s", roomID),
	}

	return rb.client.Unlink(ctx, keys...).Err()
}

// SavePKSession saves the active PK session linked to both rooms in Redis
func (rb *RedisBroker) SavePKSession(ctx context.Context, session *models.PKSession) error {
	if rb == nil || !rb.IsActive() || session == nil {
		return errors.New("redis broker is not active or session is nil")
	}

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal pk session: %w", err)
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	key1 := fmt.Sprintf("pk:session:%s", session.RoomID1)
	key2 := fmt.Sprintf("pk:session:%s", session.RoomID2)

	pipe := rb.client.Pipeline()
	pipe.Set(ctx, key1, data, 2*time.Hour)
	pipe.Set(ctx, key2, data, 2*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

// GetPKSession retrieves the active PK session for a room from Redis
func (rb *RedisBroker) GetPKSession(ctx context.Context, roomID string) (*models.PKSession, error) {
	if rb == nil || !rb.IsActive() {
		return nil, errors.New("redis broker is not active")
	}

	key := fmt.Sprintf("pk:session:%s", roomID)
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	data, err := rb.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}

	var session models.PKSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// DeletePKSession removes PK session records for both rooms from Redis
func (rb *RedisBroker) DeletePKSession(ctx context.Context, roomID1, roomID2 string) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	key1 := fmt.Sprintf("pk:session:%s", roomID1)
	key2 := fmt.Sprintf("pk:session:%s", roomID2)

	return rb.client.Del(ctx, key1, key2).Err()
}

// PublishPKEvent publishes a signaling event to the combined PK session channel
func (rb *RedisBroker) PublishPKEvent(sessionID string, msg *models.SignalingMessage) error {
	if rb == nil || !rb.IsActive() {
		return errors.New("redis broker is not active")
	}

	channel := fmt.Sprintf("pk:events:%s", sessionID)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal pk message: %w", err)
	}

	ctx, cancel := context.WithTimeout(rb.ctx, 2*time.Second)
	defer cancel()

	return rb.client.Publish(ctx, channel, data).Err()
}

// PushChatMessage stores a recent chat message in a bounded Redis list (max 50 items) with LTRIM
func (rb *RedisBroker) PushChatMessage(ctx context.Context, roomID string, msg *models.SignalingMessage) error {
	if rb == nil || !rb.IsActive() || msg == nil {
		return nil
	}

	key := fmt.Sprintf("room:%s:chats", roomID)
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	pipe := rb.client.Pipeline()
	pipe.LPush(ctx, key, data)
	pipe.LTrim(ctx, key, 0, 49) // Bounded Memory: Strictly keep only the last 50 messages (0..49)
	pipe.Expire(ctx, key, 24*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

// RefreshRoomTTL refreshes the 24-hour TTL on all Redis keys associated with an active room
func (rb *RedisBroker) RefreshRoomTTL(ctx context.Context, roomID string) error {
	if rb == nil || !rb.IsActive() {
		return nil
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 2*time.Second)
		defer cancel()
	}

	stateKey := fmt.Sprintf("room:%s:state", roomID)
	scoreKey := fmt.Sprintf("room:%s:score", roomID)
	chatsKey := fmt.Sprintf("room:%s:chats", roomID)
	originKey := fmt.Sprintf("room:%s:origin", roomID)
	nodeMapKey := fmt.Sprintf("room_node_map:%s", roomID)

	pipe := rb.client.Pipeline()
	pipe.Expire(ctx, stateKey, 24*time.Hour)
	pipe.Expire(ctx, scoreKey, 24*time.Hour)
	pipe.Expire(ctx, chatsKey, 24*time.Hour)
	pipe.Expire(ctx, originKey, 24*time.Hour)
	pipe.Expire(ctx, nodeMapKey, 24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

// BatchRefreshRoomTTLs refreshes the 24-hour TTL across a slice of active room IDs concurrently
func (rb *RedisBroker) BatchRefreshRoomTTLs(ctx context.Context, roomIDs []string) error {
	if rb == nil || !rb.IsActive() || len(roomIDs) == 0 {
		return nil
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(rb.ctx, 3*time.Second)
		defer cancel()
	}

	pipe := rb.client.Pipeline()
	for _, roomID := range roomIDs {
		stateKey := fmt.Sprintf("room:%s:state", roomID)
		scoreKey := fmt.Sprintf("room:%s:score", roomID)
		chatsKey := fmt.Sprintf("room:%s:chats", roomID)
		originKey := fmt.Sprintf("room:%s:origin", roomID)
		nodeMapKey := fmt.Sprintf("room_node_map:%s", roomID)

		pipe.Expire(ctx, stateKey, 24*time.Hour)
		pipe.Expire(ctx, scoreKey, 24*time.Hour)
		pipe.Expire(ctx, chatsKey, 24*time.Hour)
		pipe.Expire(ctx, originKey, 24*time.Hour)
		pipe.Expire(ctx, nodeMapKey, 24*time.Hour)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// Close closes all active subscriptions and the Redis client connection
func (rb *RedisBroker) Close() error {
	if rb == nil {
		return nil
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.isActive = false
	rb.cancel()

	for roomID, pubsub := range rb.subscribers {
		if pubsub != nil {
			_ = pubsub.Close()
		}
		delete(rb.subscribers, roomID)
	}

	if rb.client != nil {
		err := rb.client.Close()
		log.Println("[Redis] Connection closed gracefully.")
		return err
	}

	return nil
}
