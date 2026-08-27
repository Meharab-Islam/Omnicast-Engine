package models

import (
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

// CoHostMedia holds the video and audio media tracks for an active co-host
type CoHostMedia struct {
	CoHostID   string                      `json:"cohost_id"`
	VideoTrack *webrtc.TrackLocalStaticRTP `json:"-"`
	AudioTrack *webrtc.TrackLocalStaticRTP `json:"-"`
}

// Room represents a live media broadcast session
type Room struct {
	RoomID       string                  `json:"room_id"`
	RoomName     string                  `json:"room_name"`
	HostID       string                  `json:"host_id"`
	MainSeatID   string                  `json:"main_seat_id"`
	HostScore    int                     `json:"host_score"`
	CreatedAt    time.Time               `json:"created_at"`
	HostClient   any                     `json:"-"`
	HostPC       *webrtc.PeerConnection  `json:"-"`
	HostVideoSSRC uint32                 `json:"-"`
	VideoSSRCs   map[string]uint32       `json:"-"`
	Viewers      map[string]any          `json:"-"`
	ActiveSeats  map[string]string       `json:"active_seats"`
	MediaStates  map[string]MediaState   `json:"media_states"`
	VideoTrack   *webrtc.TrackLocalStaticRTP            `json:"-"`
	VideoTracks  map[string]*webrtc.TrackLocalStaticRTP `json:"-"`
	AudioTrack   *webrtc.TrackLocalStaticRTP            `json:"-"`
	CoHostTracks   map[string]*CoHostMedia                `json:"-"`
	TrackSwitchers map[string]any                         `json:"-"`
	reconnectTimer *time.Timer                          `json:"-"`
	isReconnecting bool                                 `json:"-"`
	lastPLITime    time.Time                            `json:"-"`
	mu           sync.RWMutex
}

// NewRoom creates and initializes a new Room with default room name, current timestamp, and default main seat
func NewRoom(roomID, hostID string) *Room {
	return &Room{
		RoomID:       roomID,
		RoomName:     roomID,
		HostID:       hostID,
		MainSeatID:   hostID,
		HostScore:    0,
		CreatedAt:    time.Now().UTC(),
		Viewers:      make(map[string]any),
		ActiveSeats:  map[string]string{"0": hostID},
		MediaStates:    make(map[string]MediaState),
		VideoTracks:    make(map[string]*webrtc.TrackLocalStaticRTP),
		VideoSSRCs:     make(map[string]uint32),
		CoHostTracks:   make(map[string]*CoHostMedia),
		TrackSwitchers: make(map[string]any),
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
		HostID:       hostID,
		MainSeatID:   hostID,
		HostScore:    0,
		CreatedAt:    time.Now().UTC(),
		Viewers:      make(map[string]any),
		ActiveSeats:    map[string]string{"0": hostID},
		MediaStates:    make(map[string]MediaState),
		VideoTracks:    make(map[string]*webrtc.TrackLocalStaticRTP),
		CoHostTracks:   make(map[string]*CoHostMedia),
		TrackSwitchers: make(map[string]any),
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

	return &RoomState{
		RoomID:       r.RoomID,
		RoomName:     r.RoomName,
		HostID:       r.HostID,
		TotalViewers: len(r.Viewers),
		HostScore:    int64(r.HostScore),
		ActiveSeats:  seatsCopy,
		MediaStates:  mediaCopy,
		CreatedAt:    r.CreatedAt,
	}
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

// SendPLIThrottled sends a Picture Loss Indication (Keyframe request) to the Host,
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
	hostPC := r.HostPC
	ssrc := r.HostVideoSSRC

	if hostPC != nil && ssrc != 0 && hostPC.ConnectionState() != webrtc.PeerConnectionStateClosed {
		_ = hostPC.WriteRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{MediaSSRC: ssrc},
		})
		return true
	}
	return false
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

// GetDefaultViewerVideoTrack returns the recommended video track for new viewers (Medium 'h', then Low 'q', then Full 'f', then default VideoTrack)
func (r *Room) GetDefaultViewerVideoTrack() *webrtc.TrackLocalStaticRTP {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.VideoTracks != nil {
		if t, ok := r.VideoTracks["h"]; ok && t != nil {
			return t
		}
		if t, ok := r.VideoTracks["q"]; ok && t != nil {
			return t
		}
		if t, ok := r.VideoTracks["f"]; ok && t != nil {
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
