package models

import (
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

// CoHostMedia holds the video and audio media tracks and WebRTC PeerConnection for an active co-host
type CoHostMedia struct {
	CoHostID       string                      `json:"cohost_id"`
	VideoTrack     *webrtc.TrackLocalStaticRTP `json:"-"`
	AudioTrack     *webrtc.TrackLocalStaticRTP `json:"-"`
	PeerConnection *webrtc.PeerConnection      `json:"-"`
	VideoSSRC      uint32                      `json:"-"`
}

// Room represents a live media broadcast session
type Room struct {
	RoomID       string                  `json:"room_id"`
	RoomName     string                  `json:"room_name"`
	RoomType     string                  `json:"room_type"` // "video" or "audio"
	HostID       string                  `json:"host_id"`
	MainSeatID   string                  `json:"main_seat_id"`
	HostScore    int                     `json:"host_score"`
	CreatedAt    time.Time               `json:"created_at"`
	HostClient   any                     `json:"-"`
	HostPC       *webrtc.PeerConnection  `json:"-"`
	HostVideoSSRC uint32                 `json:"-"`
	VideoSSRCs   map[string]uint32       `json:"-"`
	Viewers      map[string]any          `json:"-"`
	Participants map[string]*Participant `json:"-"`
	ActiveSeats  map[string]string       `json:"active_seats"`
	MediaStates  map[string]MediaState   `json:"media_states"`
	PKState      *PKState                `json:"pk_state,omitempty"`
	VideoTrack   *webrtc.TrackLocalStaticRTP            `json:"-"`
	VideoTracks  map[string]*webrtc.TrackLocalStaticRTP `json:"-"`
	AudioTrack   *webrtc.TrackLocalStaticRTP            `json:"-"`
	CoHostTracks   map[string]*CoHostMedia                `json:"-"`
	TrackSwitchers map[string]any                         `json:"-"`
	PacketBuffers  map[string]any                         `json:"-"`
	bannedUsers    map[string]bool                        `json:"-"`
	participantReconnectTimers map[string]*time.Timer     `json:"-"`
	emptyRoomTimer *time.Timer                            `json:"-"`
	reconnectTimer *time.Timer                          `json:"-"`
	isReconnecting bool                                 `json:"-"`
	lastPLITime    time.Time                            `json:"-"`

	// Phase 5: Enterprise Audio Resilience
	activeSpeakerDetector any `json:"-"` // *webrtc.ActiveSpeakerDetector (stored as any to avoid circular import)
	audioForwarder        any `json:"-"` // *webrtc.AudioForwarder

	// Phase 6: Massive Fan-Out & Dynamic Viewport Management
	viewportManager  any `json:"-"` // *webrtc.ViewportManager
	fanOutDispatcher any `json:"-"` // *webrtc.FanOutDispatcher

	// Presence Batching & Throttling
	pendingJoins    []*Participant
	pendingLeaves   []string
	stopPresence    chan struct{}
	presenceStarted bool
	presenceMu      sync.Mutex

	mu sync.RWMutex
}

// NewRoom creates and initializes a new Room with default room name, current timestamp, and default main seat
func NewRoom(roomID, hostID string) *Room {
	return &Room{
		RoomID:       roomID,
		RoomName:     roomID,
		RoomType:     "video",
		HostID:       hostID,
		MainSeatID:   hostID,
		HostScore:    0,
		CreatedAt:    time.Now().UTC(),
		Viewers:      make(map[string]any),
		Participants: make(map[string]*Participant),
		ActiveSeats:  map[string]string{"0": hostID},
		MediaStates:    make(map[string]MediaState),
		VideoTracks:    make(map[string]*webrtc.TrackLocalStaticRTP),
		VideoSSRCs:     make(map[string]uint32),
		CoHostTracks:   make(map[string]*CoHostMedia),
		TrackSwitchers: make(map[string]any),
		PacketBuffers:  make(map[string]any),
		bannedUsers:    make(map[string]bool),
		participantReconnectTimers: make(map[string]*time.Timer),
		pendingJoins:   make([]*Participant, 0),
		pendingLeaves:  make([]string, 0),
		stopPresence:   make(chan struct{}),
	}
}

// NewRoomWithName creates and initializes a new Room with an explicit room name
func NewRoomWithName(roomID, roomName, hostID string) *Room {
	if roomName == "" {
		roomName = roomID
	}
	return &Room{
		RoomID:       roomID,
		RoomName:     roomName,
		RoomType:     "video",
		HostID:       hostID,
		MainSeatID:   hostID,
		HostScore:    0,
		CreatedAt:    time.Now().UTC(),
		Viewers:      make(map[string]any),
		Participants: make(map[string]*Participant),
		ActiveSeats:    map[string]string{"0": hostID},
		MediaStates:    make(map[string]MediaState),
		VideoTracks:    make(map[string]*webrtc.TrackLocalStaticRTP),
		CoHostTracks:   make(map[string]*CoHostMedia),
		TrackSwitchers: make(map[string]any),
		PacketBuffers:  make(map[string]any),
		bannedUsers:    make(map[string]bool),
		participantReconnectTimers: make(map[string]*time.Timer),
		pendingJoins:   make([]*Participant, 0),
		pendingLeaves:  make([]string, 0),
		stopPresence:   make(chan struct{}),
	}
}

// RLock acquires the read lock
func (r *Room) RLock() {
	r.mu.RLock()
}

// RUnlock releases the read lock
func (r *Room) RUnlock() {
	r.mu.RUnlock()
}

// Lock acquires the write lock
func (r *Room) Lock() {
	r.mu.Lock()
}

// Unlock releases the write lock
func (r *Room) Unlock() {
	r.mu.Unlock()
}

// SetRoomName updates the room name in a thread-safe manner
func (r *Room) SetRoomName(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.RoomName = name
}

// GetRoomName returns the room name in a thread-safe manner
func (r *Room) GetRoomName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.RoomName == "" {
		return r.RoomID
	}
	return r.RoomName
}

// SetMainSeatID updates the currently active main seat ID in a thread-safe manner
func (r *Room) SetMainSeatID(seatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.MainSeatID = seatID
}

// GetMainSeatID returns the ID of the participant in the main seat
func (r *Room) GetMainSeatID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.MainSeatID == "" {
		return r.HostID
	}
	return r.MainSeatID
}

// SetHostScore updates the host's score in a thread-safe manner
func (r *Room) SetHostScore(score int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.HostScore = int(score)
}

// AddHostScore increments the host's score and returns the new total (thread-safe)
func (r *Room) AddHostScore(points int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.HostScore += points
	return r.HostScore
}

// GetHostScore returns the current score of the host (thread-safe)
func (r *Room) GetHostScore() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.HostScore
}

// SetMediaState updates the media mute state for a user in the room (thread-safe)
func (r *Room) SetMediaState(userID string, state MediaState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.MediaStates == nil {
		r.MediaStates = make(map[string]MediaState)
	}
	r.MediaStates[userID] = state
}

// GetMediaState retrieves the media mute state for a user in the room (thread-safe)
func (r *Room) GetMediaState(userID string) (MediaState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.MediaStates == nil {
		return MediaState{}, false
	}
	state, exists := r.MediaStates[userID]
	return state, exists
}

// SetActiveSeat assigns a seat ID to a user ID (thread-safe)
func (r *Room) SetActiveSeat(seatID, userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ActiveSeats == nil {
		r.ActiveSeats = make(map[string]string)
	}
	r.ActiveSeats[seatID] = userID
}

// RemoveActiveSeat removes a user from active seats (thread-safe)
func (r *Room) RemoveActiveSeat(seatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ActiveSeats != nil {
		delete(r.ActiveSeats, seatID)
	}
}

// RemoveUserFromSeats removes any seat assigned to the specified user (thread-safe)
func (r *Room) RemoveUserFromSeats(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ActiveSeats != nil {
		for sID, uID := range r.ActiveSeats {
			if uID == userID {
				delete(r.ActiveSeats, sID)
			}
		}
	}
}

// GetActiveSeats returns a copy of active seat assignments (thread-safe)
func (r *Room) GetActiveSeats() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seatsCopy := make(map[string]string, len(r.ActiveSeats))
	for k, v := range r.ActiveSeats {
		seatsCopy[k] = v
	}
	return seatsCopy
}

// GetRoomState constructs and returns a snapshot of the RoomState (thread-safe)
func (r *Room) GetRoomState() *RoomState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seatsCopy := make(map[string]string, len(r.ActiveSeats))
	for k, v := range r.ActiveSeats {
		seatsCopy[k] = v
	}

	mediaCopy := make(map[string]MediaState, len(r.MediaStates))
	for k, v := range r.MediaStates {
		mediaCopy[k] = v
	}

	var participantsCopy []*Participant
	if r.Participants != nil {
		participantsCopy = make([]*Participant, 0, len(r.Participants))
		for _, p := range r.Participants {
			if p != nil {
				participantsCopy = append(participantsCopy, p)
			}
		}
	}

	roomType := r.RoomType
	if roomType == "" {
		roomType = "video"
	}

	var pkCopy *PKState
	if r.PKState != nil {
		copied := *r.PKState
		pkCopy = &copied
	}

	return &RoomState{
		RoomID:       r.RoomID,
		RoomName:     r.RoomName,
		RoomType:     roomType,
		HostID:       r.HostID,
		TotalViewers: len(r.Viewers),
		HostScore:    int64(r.HostScore),
		ActiveSeats:  seatsCopy,
		MediaStates:  mediaCopy,
		Participants: participantsCopy,
		PKState:      pkCopy,
		CreatedAt:    r.CreatedAt,
	}
}

// SetPKState assigns the active PK battle state
func (r *Room) SetPKState(state *PKState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.PKState = state
}

// GetPKState returns a snapshot of the active PK battle state
func (r *Room) GetPKState() *PKState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.PKState == nil {
		return nil
	}
	copied := *r.PKState
	return &copied
}

// SetRoomType sets the room media type ('video' or 'audio')
func (r *Room) SetRoomType(roomType string) {
	if roomType != "audio" && roomType != "video" {
		roomType = "video"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.RoomType = roomType
}

// GetRoomType returns the room media type ('video' or 'audio')
func (r *Room) GetRoomType() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.RoomType == "" {
		return "video"
	}
	return r.RoomType
}

// SetHostClient stores the host client reference in a thread-safe manner
func (r *Room) SetHostClient(client any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.HostClient = client
}

// GetHostClient retrieves the host client reference in a thread-safe manner
func (r *Room) GetHostClient() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.HostClient
}

// SetHostPeerConnection stores the host's WebRTC PeerConnection in a thread-safe manner
func (r *Room) SetHostPeerConnection(pc *webrtc.PeerConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.HostPC = pc
}

// GetHostPeerConnection retrieves the host's WebRTC PeerConnection in a thread-safe manner
func (r *Room) GetHostPeerConnection() *webrtc.PeerConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.HostPC
}

// SendPLIImmediate sends an immediate un-throttled Picture Loss Indication (Keyframe request) to the Host and all active Co-Hosts
func (r *Room) SendPLIImmediate() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastPLITime = time.Now()
	sent := false

	// 1. Send PLI to Main Host
	hostPC := r.HostPC
	if hostPC != nil && hostPC.ConnectionState() != webrtc.PeerConnectionStateClosed {
		packets := make([]rtcp.Packet, 0, len(r.VideoSSRCs)+1)
		if r.HostVideoSSRC != 0 {
			packets = append(packets, &rtcp.PictureLossIndication{MediaSSRC: r.HostVideoSSRC})
		}
		for _, ssrc := range r.VideoSSRCs {
			if ssrc != 0 && ssrc != r.HostVideoSSRC {
				packets = append(packets, &rtcp.PictureLossIndication{MediaSSRC: ssrc})
			}
		}
		if len(packets) == 0 {
			packets = append(packets, &rtcp.PictureLossIndication{MediaSSRC: 0})
		}
		_ = hostPC.WriteRTCP(packets)
		sent = true
	}

	// 2. Send PLI to all active Co-Hosts
	for _, coHost := range r.CoHostTracks {
		if coHost != nil && coHost.PeerConnection != nil && coHost.PeerConnection.ConnectionState() != webrtc.PeerConnectionStateClosed {
			ssrc := coHost.VideoSSRC
			_ = coHost.PeerConnection.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{MediaSSRC: ssrc},
			})
			sent = true
		}
	}

	return sent
}

// SendPLIThrottled sends a Picture Loss Indication (Keyframe request) to the Host and Co-Hosts,
// debouncing/throttling requests to maximum 1 PLI per minInterval (e.g. 1.5 seconds) per video track.
func (r *Room) SendPLIThrottled(minInterval time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.lastPLITime) < minInterval {
		// Throttled: drop duplicate PLI request
		return false
	}

	r.lastPLITime = now
	sent := false

	hostPC := r.HostPC
	if hostPC != nil && hostPC.ConnectionState() != webrtc.PeerConnectionStateClosed {
		packets := make([]rtcp.Packet, 0, len(r.VideoSSRCs)+1)
		if r.HostVideoSSRC != 0 {
			packets = append(packets, &rtcp.PictureLossIndication{MediaSSRC: r.HostVideoSSRC})
		}
		for _, ssrc := range r.VideoSSRCs {
			if ssrc != 0 && ssrc != r.HostVideoSSRC {
				packets = append(packets, &rtcp.PictureLossIndication{MediaSSRC: ssrc})
			}
		}
		if len(packets) == 0 {
			packets = append(packets, &rtcp.PictureLossIndication{MediaSSRC: 0})
		}
		_ = hostPC.WriteRTCP(packets)
		sent = true
	}

	for _, coHost := range r.CoHostTracks {
		if coHost != nil && coHost.PeerConnection != nil && coHost.PeerConnection.ConnectionState() != webrtc.PeerConnectionStateClosed {
			ssrc := coHost.VideoSSRC
			_ = coHost.PeerConnection.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{MediaSSRC: ssrc},
			})
			sent = true
		}
	}

	return sent
}

// SetHostVideoSSRC stores the host's incoming video track SSRC
func (r *Room) SetHostVideoSSRC(ssrc uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.HostVideoSSRC = ssrc
	if r.VideoSSRCs == nil {
		r.VideoSSRCs = make(map[string]uint32)
	}
	r.VideoSSRCs["default"] = ssrc
}

// GetHostVideoSSRC retrieves the host's video track SSRC
func (r *Room) GetHostVideoSSRC() uint32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.HostVideoSSRC
}

// SetVideoTrackSSRC stores the video track SSRC for a specific RID ('q', 'h', 'f', or 'default')
func (r *Room) SetVideoTrackSSRC(rid string, ssrc uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.VideoSSRCs == nil {
		r.VideoSSRCs = make(map[string]uint32)
	}
	if rid == "" {
		rid = "default"
	}
	r.VideoSSRCs[rid] = ssrc
	// If this is default, medium 'h', or host video SSRC was empty, update HostVideoSSRC
	if r.HostVideoSSRC == 0 || rid == "h" || rid == "default" {
		r.HostVideoSSRC = ssrc
	}
}

// GetVideoTrackSSRC retrieves the SSRC for a specific RID, falling back to medium 'h' or default
func (r *Room) GetVideoTrackSSRC(rid string) uint32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.VideoSSRCs != nil {
		if ssrc, exists := r.VideoSSRCs[rid]; exists && ssrc != 0 {
			return ssrc
		}
		if rid != "h" {
			if ssrc, exists := r.VideoSSRCs["h"]; exists && ssrc != 0 {
				return ssrc
			}
		}
		if ssrc, exists := r.VideoSSRCs["default"]; exists && ssrc != 0 {
			return ssrc
		}
	}
	return r.HostVideoSSRC
}

// AddViewer adds a viewer client to the room in a thread-safe manner
func (r *Room) AddViewer(clientID string, client any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Viewers[clientID] = client
}

// RemoveViewer removes a viewer client from the room in a thread-safe manner
func (r *Room) RemoveViewer(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Viewers, clientID)
}

// GetViewer fetches a viewer client by their client ID
func (r *Room) GetViewer(clientID string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.Viewers[clientID]
	return client, ok
}

// GetViewersCopy returns a shallow copy of all viewers under read lock
func (r *Room) GetViewersCopy() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copyMap := make(map[string]any, len(r.Viewers))
	for k, v := range r.Viewers {
		copyMap[k] = v
	}
	return copyMap
}

// GetViewersList returns a slice of all connected viewer client IDs under read lock
func (r *Room) GetViewersList() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]string, 0, len(r.Viewers))
	for k := range r.Viewers {
		list = append(list, k)
	}
	return list
}

// ViewersCount returns the number of active viewers
func (r *Room) ViewersCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Viewers)
}

// SetTracks sets the host's video and audio tracks
func (r *Room) SetTracks(videoTrack, audioTrack *webrtc.TrackLocalStaticRTP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.VideoTrack = videoTrack
	r.AudioTrack = audioTrack
}

// SetVideoTrack sets the host's video track in a thread-safe manner
func (r *Room) SetVideoTrack(track *webrtc.TrackLocalStaticRTP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.VideoTrack = track
	if r.VideoTracks == nil {
		r.VideoTracks = make(map[string]*webrtc.TrackLocalStaticRTP)
	}
	r.VideoTracks["default"] = track
}

// SetVideoTrackRID stores a video track for a specific simulcast RID ('q', 'h', 'f') in a thread-safe manner
func (r *Room) SetVideoTrackRID(rid string, track *webrtc.TrackLocalStaticRTP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.VideoTracks == nil {
		r.VideoTracks = make(map[string]*webrtc.TrackLocalStaticRTP)
	}
	r.VideoTracks[rid] = track
	// Set default VideoTrack for backwards compatibility (prefer medium 'h', or high 'f', or first available)
	if r.VideoTrack == nil || rid == "h" || (r.VideoTracks["h"] == nil && rid == "f") {
		r.VideoTrack = track
	}
}

// GetVideoTrackByRID retrieves a specific simulcast video track ('q', 'h', 'f'), or fallback default
func (r *Room) GetVideoTrackByRID(rid string) *webrtc.TrackLocalStaticRTP {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.VideoTracks != nil {
		if track, ok := r.VideoTracks[rid]; ok && track != nil {
			return track
		}
	}
	return r.VideoTrack
}

// GetVideoTrack returns the current default video track in a thread-safe manner
func (r *Room) GetVideoTrack() *webrtc.TrackLocalStaticRTP {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.VideoTrack
}

// GetAudioTrack returns the current audio track in a thread-safe manner
func (r *Room) GetAudioTrack() *webrtc.TrackLocalStaticRTP {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.AudioTrack
}

// GetDefaultViewerVideoTrack returns the recommended video track for new viewers (HD 'f' first, then Medium 'h', then Low 'q', then default VideoTrack)
func (r *Room) GetDefaultViewerVideoTrack() *webrtc.TrackLocalStaticRTP {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.VideoTracks != nil {
		if t, ok := r.VideoTracks["f"]; ok && t != nil {
			return t
		}
		if t, ok := r.VideoTracks["h"]; ok && t != nil {
			return t
		}
		if t, ok := r.VideoTracks["q"]; ok && t != nil {
			return t
		}
	}
	return r.VideoTrack
}

// GetAllVideoTracks returns a copy of all simulcast video tracks
func (r *Room) GetAllVideoTracks() map[string]*webrtc.TrackLocalStaticRTP {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.VideoTracks == nil {
		return make(map[string]*webrtc.TrackLocalStaticRTP)
	}
	copyMap := make(map[string]*webrtc.TrackLocalStaticRTP, len(r.VideoTracks))
	for k, v := range r.VideoTracks {
		copyMap[k] = v
	}
	return copyMap
}

// SetAudioTrack sets the host's audio track in a thread-safe manner
func (r *Room) SetAudioTrack(track *webrtc.TrackLocalStaticRTP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.AudioTrack = track
}

// SetCoHostTrack sets video track for a co-host in a thread-safe manner
func (r *Room) SetCoHostTrack(coHostID string, track *webrtc.TrackLocalStaticRTP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CoHostTracks == nil {
		r.CoHostTracks = make(map[string]*CoHostMedia)
	}
	media, exists := r.CoHostTracks[coHostID]
	if !exists || media == nil {
		media = &CoHostMedia{CoHostID: coHostID}
		r.CoHostTracks[coHostID] = media
	}
	media.VideoTrack = track
}

// SetCoHostMedia sets both video and audio tracks for a co-host
func (r *Room) SetCoHostMedia(coHostID string, media *CoHostMedia) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CoHostTracks == nil {
		r.CoHostTracks = make(map[string]*CoHostMedia)
	}
	r.CoHostTracks[coHostID] = media
}

// SetCoHostAudioTrack sets audio track for a co-host
func (r *Room) SetCoHostAudioTrack(coHostID string, track *webrtc.TrackLocalStaticRTP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CoHostTracks == nil {
		r.CoHostTracks = make(map[string]*CoHostMedia)
	}
	media, exists := r.CoHostTracks[coHostID]
	if !exists || media == nil {
		media = &CoHostMedia{CoHostID: coHostID}
		r.CoHostTracks[coHostID] = media
	}
	media.AudioTrack = track
}

// SetCoHostPeerConnection assigns the PeerConnection for a co-host
func (r *Room) SetCoHostPeerConnection(coHostID string, pc *webrtc.PeerConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CoHostTracks == nil {
		r.CoHostTracks = make(map[string]*CoHostMedia)
	}
	media, exists := r.CoHostTracks[coHostID]
	if !exists || media == nil {
		media = &CoHostMedia{CoHostID: coHostID}
		r.CoHostTracks[coHostID] = media
	}
	media.PeerConnection = pc
}

// GetCoHostPeerConnection retrieves the PeerConnection for a specific co-host in a thread-safe manner
func (r *Room) GetCoHostPeerConnection(coHostID string) *webrtc.PeerConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.CoHostTracks == nil {
		return nil
	}
	if media, exists := r.CoHostTracks[coHostID]; exists && media != nil {
		return media.PeerConnection
	}
	return nil
}

// SetCoHostVideoSSRC assigns the incoming video SSRC for a co-host
func (r *Room) SetCoHostVideoSSRC(coHostID string, ssrc uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CoHostTracks == nil {
		r.CoHostTracks = make(map[string]*CoHostMedia)
	}
	media, exists := r.CoHostTracks[coHostID]
	if !exists || media == nil {
		media = &CoHostMedia{CoHostID: coHostID}
		r.CoHostTracks[coHostID] = media
	}
	media.VideoSSRC = ssrc
}

// GetCoHostTrack retrieves a specific co-host's video track
func (r *Room) GetCoHostTrack(coHostID string) (*webrtc.TrackLocalStaticRTP, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.CoHostTracks == nil {
		return nil, false
	}
	media, ok := r.CoHostTracks[coHostID]
	if !ok || media == nil || media.VideoTrack == nil {
		return nil, false
	}
	return media.VideoTrack, true
}

// GetCoHostMedia retrieves the full CoHostMedia for a co-host
func (r *Room) GetCoHostMedia(coHostID string) (*CoHostMedia, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.CoHostTracks == nil {
		return nil, false
	}
	media, ok := r.CoHostTracks[coHostID]
	return media, ok
}

// GetActiveCoHostIDs returns a list of IDs of all currently active co-hosts
func (r *Room) GetActiveCoHostIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.CoHostTracks == nil {
		return []string{}
	}
	ids := make([]string, 0, len(r.CoHostTracks))
	for id := range r.CoHostTracks {
		ids = append(ids, id)
	}
	return ids
}

// RegisterTrackSwitcher associates a viewer's TrackSwitcher with the room
func (r *Room) RegisterTrackSwitcher(viewerID string, switcher any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.TrackSwitchers == nil {
		r.TrackSwitchers = make(map[string]any)
	}
	r.TrackSwitchers[viewerID] = switcher
}

// UnregisterTrackSwitcher removes a viewer's TrackSwitcher from the room
func (r *Room) UnregisterTrackSwitcher(viewerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.TrackSwitchers != nil {
		delete(r.TrackSwitchers, viewerID)
	}
}

// GetAllTrackSwitchers returns a snapshot copy of all active track switchers
func (r *Room) GetAllTrackSwitchers() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.TrackSwitchers == nil {
		return make(map[string]any)
	}
	copyMap := make(map[string]any, len(r.TrackSwitchers))
	for k, v := range r.TrackSwitchers {
		copyMap[k] = v
	}
	return copyMap
}

// GetTrackSwitcher retrieves a specific viewer's TrackSwitcher
func (r *Room) GetTrackSwitcher(viewerID string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.TrackSwitchers == nil {
		return nil, false
	}
	switcher, exists := r.TrackSwitchers[viewerID]
	return switcher, exists
}

// SetPacketBuffer stores a track's PacketBuffer in a thread-safe manner
func (r *Room) SetPacketBuffer(layerKey string, pb any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.PacketBuffers == nil {
		r.PacketBuffers = make(map[string]any)
	}
	r.PacketBuffers[layerKey] = pb
}

// GetPacketBuffer retrieves a specific layer's PacketBuffer
func (r *Room) GetPacketBuffer(layerKey string) any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.PacketBuffers == nil {
		return nil
	}
	return r.PacketBuffers[layerKey]
}

// GetAllPacketBuffers returns a copy of all PacketBuffers
func (r *Room) GetAllPacketBuffers() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.PacketBuffers == nil {
		return make(map[string]any)
	}
	copyMap := make(map[string]any, len(r.PacketBuffers))
	for k, v := range r.PacketBuffers {
		copyMap[k] = v
	}
	return copyMap
}

// GetAllCoHostTracks returns a shallow copy of all registered co-host media
func (r *Room) GetAllCoHostTracks() map[string]*CoHostMedia {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.CoHostTracks == nil {
		return make(map[string]*CoHostMedia)
	}
	copyMap := make(map[string]*CoHostMedia, len(r.CoHostTracks))
	for k, v := range r.CoHostTracks {
		copyMap[k] = v
	}
	return copyMap
}

// RemoveCoHostTrack deletes a co-host when they leave the co-host seat
func (r *Room) RemoveCoHostTrack(coHostID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CoHostTracks != nil {
		delete(r.CoHostTracks, coHostID)
	}
}

// StartReconnectTimer starts a grace period timer for host reconnection
func (r *Room) StartReconnectTimer(d time.Duration, onTimeout func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reconnectTimer != nil {
		r.reconnectTimer.Stop()
	}
	r.isReconnecting = true
	r.reconnectTimer = time.AfterFunc(d, func() {
		r.mu.Lock()
		r.isReconnecting = false
		r.reconnectTimer = nil
		r.mu.Unlock()
		if onTimeout != nil {
			onTimeout()
		}
	})
}

// CancelReconnectTimer cancels the pending grace period timer when the host reconnects
func (r *Room) CancelReconnectTimer() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reconnectTimer != nil {
		stopped := r.reconnectTimer.Stop()
		r.reconnectTimer = nil
		r.isReconnecting = false
		return stopped
	}
	wasReconnecting := r.isReconnecting
	r.isReconnecting = false
	return wasReconnecting
}

// IsReconnecting returns whether the room is currently waiting for host reconnection during grace period
func (r *Room) IsReconnecting() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isReconnecting
}

// AddBannedUser adds a user ID to the room's permanent blacklist
func (r *Room) AddBannedUser(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bannedUsers == nil {
		r.bannedUsers = make(map[string]bool)
	}
	r.bannedUsers[userID] = true
}

// IsUserBanned checks if a user is currently banned from entering the room
func (r *Room) IsUserBanned(userID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.bannedUsers == nil {
		return false
	}
	return r.bannedUsers[userID]
}

// StartParticipantReconnectTimer starts a grace period timer for a disconnected viewer or co-host
func (r *Room) StartParticipantReconnectTimer(userID string, d time.Duration, onTimeout func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.participantReconnectTimers == nil {
		r.participantReconnectTimers = make(map[string]*time.Timer)
	}
	if existing, ok := r.participantReconnectTimers[userID]; ok && existing != nil {
		existing.Stop()
	}
	r.participantReconnectTimers[userID] = time.AfterFunc(d, func() {
		r.mu.Lock()
		delete(r.participantReconnectTimers, userID)
		r.mu.Unlock()
		if onTimeout != nil {
			onTimeout()
		}
	})
}

// CancelParticipantReconnectTimer stops a participant's reconnect timer when they restore connection
func (r *Room) CancelParticipantReconnectTimer(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.participantReconnectTimers == nil {
		return false
	}
	if timer, ok := r.participantReconnectTimers[userID]; ok && timer != nil {
		stopped := timer.Stop()
		delete(r.participantReconnectTimers, userID)
		return stopped
	}
	return false
}

// StartEmptyRoomTimer starts a delayed auto-destruction timer when a room becomes empty
func (r *Room) StartEmptyRoomTimer(d time.Duration, onTimeout func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.emptyRoomTimer != nil {
		r.emptyRoomTimer.Stop()
	}
	r.emptyRoomTimer = time.AfterFunc(d, func() {
		r.mu.Lock()
		r.emptyRoomTimer = nil
		r.mu.Unlock()
		if onTimeout != nil {
			onTimeout()
		}
	})
}

// CancelEmptyRoomTimer cancels the empty room destruction timer when someone joins
func (r *Room) CancelEmptyRoomTimer() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.emptyRoomTimer != nil {
		stopped := r.emptyRoomTimer.Stop()
		r.emptyRoomTimer = nil
		return stopped
	}
	return false
}

// AddParticipant stores a participant's profile metadata in the room
func (r *Room) AddParticipant(p *Participant) {
	if p == nil || p.UserID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Participants == nil {
		r.Participants = make(map[string]*Participant)
	}
	r.Participants[p.UserID] = p
}

// RemoveParticipant removes a participant's profile metadata from the room
func (r *Room) RemoveParticipant(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Participants != nil {
		delete(r.Participants, userID)
	}
}

// GetParticipant retrieves a specific participant's profile metadata
func (r *Room) GetParticipant(userID string) (*Participant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Participants == nil {
		return nil, false
	}
	p, exists := r.Participants[userID]
	return p, exists
}

// GetParticipantsList returns a snapshot slice of all active participants in the room
func (r *Room) GetParticipantsList() []*Participant {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Participants == nil {
		return nil
	}
	list := make([]*Participant, 0, len(r.Participants))
	for _, p := range r.Participants {
		if p != nil {
			list = append(list, p)
		}
	}
	return list
}

// EnqueuePresenceJoin enqueues a joined participant into the presence batch queue
func (r *Room) EnqueuePresenceJoin(p *Participant) {
	if p == nil {
		return
	}
	r.presenceMu.Lock()
	defer r.presenceMu.Unlock()
	r.pendingJoins = append(r.pendingJoins, p)
}

// EnqueuePresenceLeave enqueues a departed user into the presence batch queue
func (r *Room) EnqueuePresenceLeave(userID string) {
	if userID == "" {
		return
	}
	r.presenceMu.Lock()
	defer r.presenceMu.Unlock()
	r.pendingLeaves = append(r.pendingLeaves, userID)
}

// FlushPresence drains and returns the accumulated joins and leaves
func (r *Room) FlushPresence() (joins []*Participant, leaves []string, totalCount int, participants []*Participant) {
	r.presenceMu.Lock()
	if len(r.pendingJoins) == 0 && len(r.pendingLeaves) == 0 {
		r.presenceMu.Unlock()
		return nil, nil, r.ViewersCount(), nil
	}
	joins = r.pendingJoins
	leaves = r.pendingLeaves
	r.pendingJoins = make([]*Participant, 0)
	r.pendingLeaves = make([]string, 0)
	r.presenceMu.Unlock()

	totalCount = r.ViewersCount()
	participants = r.GetParticipantsList()
	return joins, leaves, totalCount, participants
}

// StartPresenceBatcher runs a background ticker (e.g. 1s) to batch broadcast presence events
func (r *Room) StartPresenceBatcher(interval time.Duration, onFlush func(joins []*Participant, leaves []string, count int, list []*Participant)) {
	r.presenceMu.Lock()
	if r.presenceStarted {
		r.presenceMu.Unlock()
		return
	}
	r.presenceStarted = true
	stopCh := r.stopPresence
	r.presenceMu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				joins, leaves, count, list := r.FlushPresence()
				if (len(joins) > 0 || len(leaves) > 0) && onFlush != nil {
					onFlush(joins, leaves, count, list)
				}
			}
		}
	}()
}

// StopPresenceBatcher stops the presence batching background routine
func (r *Room) StopPresenceBatcher() {
	r.presenceMu.Lock()
	defer r.presenceMu.Unlock()
	if r.presenceStarted {
		r.presenceStarted = false
		select {
		case <-r.stopPresence:
		default:
			close(r.stopPresence)
		}
	}
}

// GetActiveSpeakerDetector returns the active speaker detector instance (as any)
func (r *Room) GetActiveSpeakerDetector() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeSpeakerDetector
}

// SetActiveSpeakerDetector sets the active speaker detector instance
func (r *Room) SetActiveSpeakerDetector(detector any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeSpeakerDetector = detector
}

// GetAudioForwarder returns the audio forwarder instance (as any)
func (r *Room) GetAudioForwarder() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.audioForwarder
}

// SetAudioForwarder sets the audio forwarder instance
func (r *Room) SetAudioForwarder(forwarder any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audioForwarder = forwarder
}

// GetViewportManager returns the viewport manager instance (as any)
func (r *Room) GetViewportManager() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.viewportManager
}

// SetViewportManager sets the viewport manager instance
func (r *Room) SetViewportManager(vm any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.viewportManager = vm
}

// GetFanOutDispatcher returns the fan-out dispatcher instance (as any)
func (r *Room) GetFanOutDispatcher() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fanOutDispatcher
}

// SetFanOutDispatcher sets the fan-out dispatcher instance
func (r *Room) SetFanOutDispatcher(d any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fanOutDispatcher = d
}


