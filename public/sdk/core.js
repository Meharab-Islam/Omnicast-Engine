/**
 * @file core.js
 * @package LiveMediaSDK
 * @version 1.0.0
 * @author Live Media Server Team
 * 
 * ============================================================================
 * ARCHITECTURE OVERVIEW & CROSS-PLATFORM SPECIFICATION
 * ============================================================================
 * 
 * This module is a pure, headless, UI-independent core SDK client for the Go
 * Live Media Server. It contains ZERO browser DOM bindings (no document, no window,
 * no video element manipulation).
 * 
 * Cross-Platform Implementation Equivalents:
 * ----------------------------------------------------------------------------
 * 1. Dart (Flutter):
 *    - Class: `class LiveMediaCore extends ChangeNotifier` or `StreamController`
 *    - WebRTC: `flutter_webrtc` (RTCPeerConnection, MediaStream, MediaStreamTrack)
 *    - WebSocket: `web_socket_channel` (IOWebSocketChannel / WebSocketChannel)
 * 
 * 2. TypeScript / JavaScript (React Native / Node.js / Web):
 *    - Class: `export class LiveMediaCore extends EventEmitter`
 *    - WebRTC: `react-native-webrtc` or standard WebRTC API
 *    - WebSocket: `ws` or standard WebSocket API
 * 
 * 3. Swift (iOS):
 *    - Class: `class LiveMediaCore: ObservableObject` / `LiveMediaCoreDelegate`
 *    - WebRTC: `WebRTC.framework` (RTCPeerConnection, RTCMediaStream)
 *    - WebSocket: `URLSessionWebSocketTask` or `Starscream`
 * ============================================================================
 */

/**
 * Lightweight, zero-dependency EventEmitter for cross-platform event dispatching
 */
class EventEmitter {
  constructor() {
    this._listeners = new Map();
  }

  /**
   * Subscribe to an event
   * @param {string} event - Event name
   * @param {Function} handler - Callback handler
   */
  on(event, handler) {
    if (!this._listeners.has(event)) {
      this._listeners.set(event, new Set());
    }
    this._listeners.get(event).add(handler);
    return () => this.off(event, handler);
  }

  /**
   * Subscribe to an event once
   * @param {string} event - Event name
   * @param {Function} handler - Callback handler
   */
  once(event, handler) {
    const wrapper = (...args) => {
      this.off(event, wrapper);
      handler(...args);
    };
    return this.on(event, wrapper);
  }

  /**
   * Unsubscribe from an event
   * @param {string} event - Event name
   * @param {Function} handler - Callback handler
   */
  off(event, handler) {
    if (this._listeners.has(event)) {
      this._listeners.get(event).delete(handler);
      if (this._listeners.get(event).size === 0) {
        this._listeners.delete(event);
      }
    }
  }

  /**
   * Emit an event to all registered handlers
   * @param {string} event - Event name
   * @param  {...any} args - Event arguments
   */
  emit(event, ...args) {
    if (this._listeners.has(event)) {
      this._listeners.get(event).forEach(handler => {
        try {
          handler(...args);
        } catch (error) {
          console.error(`[EventEmitter] Error in event '${event}' handler:`, error);
        }
      });
    }
  }

  /**
   * Remove all registered listeners
   */
  removeAllListeners() {
    this._listeners.clear();
  }
}

/**
 * LiveMediaCore - Headless WebRTC & Signaling Client
 * 
 * Dispatched Events:
 * ----------------------------------------------------------------------------
 *  - 'onConnected'            ()
 *  - 'onDisconnected'         (event)
 *  - 'onError'                (error)
 *  - 'onTrack'                ({ track, stream, streams, kind })
 *  - 'onConnectionStateChange' (state)
 *  - 'onIceStateChange'       (iceState)
 *  - 'onChat'                 ({ userId, text, raw })
 *  - 'onGiftReceived'         ({ senderId, coins, newScore, raw })
 *  - 'onViewerCount'          (count)
 *  - 'onRoomInfo'             ({ roomId, hostId, hostScore, mainSeatId, activeCohosts, viewerCount, createdAt })
 *  - 'onMainSeatChanged'      ({ targetId, mainSeatId, raw })
 *  - 'onNewCoHost'            ({ cohostId, raw })
 *  - 'onSeatRequest'          ({ requesterId, raw })
 *  - 'onSeatAccept'           ({ targetUser, raw })
 *  - 'onRoomClosed'           ({ roomId, raw })
 * ----------------------------------------------------------------------------
 */
class LiveMediaCore extends EventEmitter {
  /**
   * @param {Object} options - Configuration options
   * @param {string} options.wsUrl - WebSocket URL (e.g. ws://localhost:8080/ws)
   * @param {string} [options.token] - JWT Authentication Token
   * @param {string} [options.roomId] - Default Room ID (e.g. room-101)
   * @param {string} [options.userId] - Client User ID
   * @param {string} [options.role='viewer'] - Client Role: 'host', 'viewer', 'cohost'
   * @param {RTCConfiguration} [options.rtcConfig] - WebRTC STUN/TURN configuration
   */
  constructor(options = {}) {
    super();

    this.wsUrl = options.wsUrl || '';
    this.token = options.token || '';
    this.roomId = options.roomId || 'default-room';
    this.userId = options.userId || 'user-' + Math.random().toString(36).substring(2, 8);
    this.role = options.role || 'viewer';

    if (options.iceServers && Array.isArray(options.iceServers)) {
      this.rtcConfig = { iceServers: options.iceServers };
    } else {
      this.rtcConfig = options.rtcConfig || {
        iceServers: [
          { urls: 'stun:stun.l.google.com:19302' },
          { urls: 'stun:stun1.l.google.com:19302' }
        ]
      };
    }

    /** @type {WebSocket|null} */
    this.ws = null;

    /** @type {RTCPeerConnection|null} */
    this.peerConnection = null;

    /** @type {MediaStream|null} */
    this.localStream = null;

    /** @type {MediaStream|null} */
    this.remoteStream = null;

    this.isConnected = false;
    this.isDisposed = false;
  }

  // ==========================================================================
  // 1. WEBSOCKET & SIGNALING CONNECTION
  // ==========================================================================

  /**
   * Connects to the signaling server via WebSocket with JWT authentication
   * @returns {Promise<void>}
   */
  connect() {
    if (this.isDisposed) {
      return Promise.reject(new Error('LiveMediaCore instance is disposed'));
    }

    return new Promise((resolve, reject) => {
      try {
        let fullWsUrl = this.wsUrl;
        if (this.token) {
          const separator = fullWsUrl.includes('?') ? '&' : '?';
          fullWsUrl += `${separator}token=${encodeURIComponent(this.token)}`;
        }

        this.ws = new WebSocket(fullWsUrl);

        this.ws.onopen = () => {
          this.isConnected = true;
          this.emit('onConnected');
          resolve();
        };

        this.ws.onclose = (event) => {
          this.isConnected = false;
          this.emit('onDisconnected', event);
        };

        this.ws.onerror = (error) => {
          this.emit('onError', error);
          if (!this.isConnected) {
            reject(error);
          }
        };

        this.ws.onmessage = (event) => {
          this._handleSignalingRawMessage(event.data);
        };

      } catch (err) {
        this.emit('onError', err);
        reject(err);
      }
    });
  }

  /**
   * Internal message router that parses JSON and invokes corresponding handlers
   * @param {string} rawData - WebSocket raw string message
   * @private
   */
  async _handleSignalingRawMessage(rawData) {
    let msg;
    try {
      msg = JSON.parse(rawData);
    } catch (e) {
      this.emit('onError', new Error('Failed to parse signaling JSON: ' + e.message));
      return;
    }

    const event = msg.event;
    let payload = msg.payload;

    // Normalize stringified JSON payload if necessary
    if (typeof payload === 'string') {
      try {
        payload = JSON.parse(payload);
      } catch (_) {}
    }

    switch (event) {
      // 1. SDP Offer from server (WebRTC Renegotiation for Multi-Guest / Co-Host)
      case 'offer':
      case 'sdp_offer':
        if (this.peerConnection && this.peerConnection.connectionState !== 'closed' && payload) {
          try {
            await this.peerConnection.setRemoteDescription(new RTCSessionDescription(payload));
            const answer = await this.peerConnection.createAnswer();
            await this.peerConnection.setLocalDescription(answer);

            // Send renegotiated SDP answer back to server
            this.send({
              event: 'sdp_answer',
              room_id: this.roomId,
              user_id: this.userId,
              payload: answer
            });
            console.log('[LiveMediaSDK] Handled renegotiation SDP offer and replied with SDP answer');
          } catch (err) {
            console.error('[LiveMediaSDK] Failed to handle renegotiation SDP offer:', err);
            this.emit('onError', new Error('Renegotiation failed: ' + err.message));
          }
        }
        break;

      // 2. SDP Answer from server
      case 'answer':
      case 'sdp_answer':
        if (this.peerConnection && this.peerConnection.connectionState !== 'closed' && payload) {
          try {
            await this.peerConnection.setRemoteDescription(new RTCSessionDescription(payload));
          } catch (err) {
            if (this.peerConnection && this.peerConnection.connectionState !== 'closed') {
              this.emit('onError', new Error('SetRemoteDescription failed: ' + err.message));
            }
          }
        }
        break;

      // 2. Remote ICE Candidate from server
      case 'ice':
        if (this.peerConnection && this.peerConnection.connectionState !== 'closed' && payload) {
          try {
            await this.peerConnection.addIceCandidate(new RTCIceCandidate(payload));
          } catch (err) {
            // Silently ignore late ICE candidates if connection was closed
            if (this.peerConnection && this.peerConnection.connectionState !== 'closed') {
              this.emit('onError', new Error('AddIceCandidate failed: ' + err.message));
            }
          }
        }
        break;

      // 3. Live Room Chat
      case 'chat':
      case 'chat_message':
        this.emit('onChat', {
          userId: msg.user_id || 'anonymous',
          text: payload ? (payload.text || payload) : '',
          raw: msg
        });
        break;

      // 4. Gift Processed & Received
      case 'gift_processed':
      case 'gift_received':
        this.emit('onGiftProcessed', {
          sender: payload ? (payload.sender || payload.sender_id) : msg.user_id,
          senderId: payload ? (payload.sender || payload.sender_id) : msg.user_id,
          coins: payload ? (payload.coins || 10) : 10,
          newScore: payload ? payload.new_score : 0,
          giftId: payload ? payload.gift_id : 'rose',
          raw: msg
        });
        this.emit('onGiftReceived', {
          senderId: payload ? (payload.sender || payload.sender_id) : msg.user_id,
          coins: payload ? (payload.coins || 10) : 10,
          newScore: payload ? payload.new_score : 0,
          raw: msg
        });
        break;

      // 5. Live Viewer Count & State Update
      case 'viewer_update':
        const total = (payload && (payload.total_viewers !== undefined ? payload.total_viewers : payload.count)) || msg.total_viewers || (msg.count || 0);
        const list = (payload && payload.viewers_list) || msg.viewers_list || [];
        this.emit('onViewerUpdate', {
          totalViewers: total,
          viewersList: list,
          raw: msg
        });
        this.emit('onViewerCount', total);
        break;

      case 'viewer_count':
        const count = payload && payload.count !== undefined ? payload.count : (msg.count || 0);
        this.emit('onViewerCount', count);
        break;

      // 6. Late Joiner Room State & Info Sync
      case 'room_info_sync':
      case 'room_info':
        if (payload) {
          const roomState = {
            roomId: payload.room_id || msg.room_id,
            roomName: payload.room_name || '',
            hostId: payload.host_id,
            totalViewers: payload.total_viewers !== undefined ? payload.total_viewers : (payload.viewer_count || 0),
            hostScore: payload.host_score || 0,
            activeSeats: payload.active_seats || {},
            mediaStates: payload.media_states || {},
            createdAt: payload.created_at,
            raw: msg
          };
          this.emit('onRoomInfoSync', roomState);
          this.emit('onRoomInfo', roomState);
        }
        break;

      // 7. Track Muted / Media State Updated (Mute / Unmute)
      case 'track_muted':
      case 'media_state_updated':
        if (payload) {
          const isMuted = payload.muted !== undefined ? payload.muted : (payload.muted_video || false);
          const kind = payload.kind || (payload.muted_audio !== undefined ? 'audio' : 'video');
          const userId = payload.user_id || msg.user_id;
          const trackId = payload.track_id || `${userId}_${kind}`;

          this.emit('onTrackMuted', {
            type: 'track_muted',
            trackId: trackId,
            muted: isMuted,
            kind: kind,
            userId: userId,
            mutedAudio: payload.muted_audio || false,
            mutedVideo: payload.muted_video || false,
            mediaStates: payload.media_states || {},
            raw: msg
          });
          this.emit('onMediaStateUpdated', {
            userId: userId,
            mutedAudio: payload.muted_audio || false,
            mutedVideo: payload.muted_video || false,
            mediaStates: payload.media_states || {},
            raw: msg
          });
        }
        break;

      // 7. Main Seat Changed
      case 'main_seat_changed':
        const targetId = payload ? (payload.target_id || payload.main_seat_id) : msg.target_user;
        this.emit('onMainSeatChanged', {
          targetId: targetId,
          mainSeatId: targetId,
          raw: msg
        });
        break;

      // 8. New Co-Host Joined
      case 'new_cohost':
        this.emit('onNewCoHost', {
          cohostId: payload ? payload.cohost_id : msg.user_id,
          raw: msg
        });
        break;

      // 9. Co-Host Seat Request (received by Host)
      case 'seat_request':
        this.emit('onSeatRequest', {
          requesterId: msg.user_id,
          raw: msg
        });
        break;

      // 10. Co-Host Seat Accepted (received by Viewer)
      case 'seat_accept':
        this.emit('onSeatAccept', {
          targetUser: msg.target_user || (payload && payload.target_user),
          seatId: payload ? payload.seat_id : undefined,
          raw: msg
        });
        break;

      // 11. Seat Updated (Join, Leave, Kick)
      case 'seat_updated':
        if (payload) {
          this.emit('onSeatUpdated', {
            action: payload.action,
            userId: payload.user_id || msg.user_id,
            seatId: payload.seat_id,
            activeSeats: payload.active_seats || {},
            raw: msg
          });
        }
        break;

      // 12. Voluntary Seat Left
      case 'seat_left':
        this.emit('onSeatLeft', {
          userId: payload ? payload.user_id : msg.user_id,
          raw: msg
        });
        break;

      // 13. Host Kicked from Seat
      case 'seat_kicked':
        this.emit('onSeatKicked', {
          userId: payload ? payload.user_id : msg.user_id,
          raw: msg
        });
        break;

      // 14. Co-Host Left Room/Seat
      case 'cohost_left':
        this.emit('onCoHostLeft', {
          cohostId: payload ? (payload.cohost_id || payload.user_id) : msg.user_id,
          raw: msg
        });
        break;

      // 15. PK Battle Invitation (received by Host B)
      case 'pk_request':
        this.emit('onPKRequest', {
          fromRoomId: payload ? (payload.from_room_id || payload.room_a_id) : msg.room_id,
          fromHostId: payload ? payload.from_host_id : msg.user_id,
          raw: msg
        });
        break;

      // 16. PK Battle Started
      case 'pk_started':
        this.emit('onPKStarted', {
          sessionId: payload ? payload.session_id : undefined,
          room1: payload ? payload.room_1 : undefined,
          room2: payload ? payload.room_2 : undefined,
          host1: payload ? payload.host_1 : undefined,
          host2: payload ? payload.host_2 : undefined,
          score1: payload ? payload.host_1_score : 0,
          score2: payload ? payload.host_2_score : 0,
          raw: msg
        });
        break;

      // 17. PK Score Update (Real-time Cross-Room Sync)
      case 'pk_score_update':
        this.emit('onPKScoreUpdate', {
          sessionId: payload ? payload.session_id : undefined,
          room1: payload ? (payload.room_a_id || payload.room_1) : undefined,
          room2: payload ? (payload.room_b_id || payload.room_2) : undefined,
          score1: payload ? (payload.room_a_score ?? payload.score_1 ?? payload.host_1_score) : 0,
          score2: payload ? (payload.room_b_score ?? payload.score_2 ?? payload.host_2_score) : 0,
          raw: msg
        });
        break;

      // 18. PK Battle Ended
      case 'pk_ended':
        this.emit('onPKEnded', {
          sessionId: payload ? payload.session_id : undefined,
          raw: msg
        });
        break;

      // 19. Room Closed by Host
      case 'room_closed':
        this.emit('onRoomClosed', {
          roomId: msg.room_id,
          raw: msg
        });
        break;

      // 20. Dynacast Layer Pause (Host Upstream Bandwidth Saving)
      case 'dynacast_pause_layer': {
        const layer = payload ? (payload.layer || payload.rid) : (msg.layer || msg.rid);
        this._setLayerActive(layer, false);
        this.emit('onDynacastLayerPaused', { layer, raw: msg });
        break;
      }

      // 21. Dynacast Layer Resume
      case 'dynacast_resume_layer': {
        const layer = payload ? (payload.layer || payload.rid) : (msg.layer || msg.rid);
        this._setLayerActive(layer, true);
        this.emit('onDynacastLayerResumed', { layer, raw: msg });
        break;
      }

      // 12. Host Reconnecting (Grace Period)
      case 'host_reconnecting':
        this.emit('onHostReconnecting', {
          roomId: msg.room_id,
          raw: msg
        });
        break;

      // 13. Host Reconnected
      case 'host_reconnected':
        this.emit('onHostReconnected', {
          roomId: msg.room_id,
          raw: msg
        });
        break;

      // 14. Server Error Message
      case 'error':
        this.emit('onError', new Error(payload ? (payload.message || payload.error || 'Server error') : 'Server error'));
        break;

      default:
        // Generic event forwarding
        this.emit(event, payload, msg);
        break;
    }
  }

  /**
   * Send a formatted signaling message through WebSocket
   * @param {string} event - Event name
   * @param {Object} [payload={}] - Payload data
   * @param {string} [targetUser=''] - Optional target user
   */
  sendSignalingMessage(event, payload = {}, targetUser = '') {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket is not connected');
    }

    const message = {
      event: event,
      room_id: this.roomId,
      user_id: this.userId,
      target_user: targetUser,
      payload: payload
    };

    this.ws.send(JSON.stringify(message));
  }

  // ==========================================================================
  // 2. WEBRTC MEDIA BROADCASTING & SUBSCRIPTION
  // ==========================================================================

  /**
   * Fetches dynamic STUN/TURN ICE servers configuration from the server (/turn_credentials or /api/ice-servers)
   * and injects them into the RTCPeerConnection config, ensuring seamless fallback to TCP/Relay if direct UDP fails.
   * @returns {Promise<RTCConfiguration>}
   */
  async fetchIceServers() {
    try {
      const uid = this.currentUserID || `user-${Date.now()}`;
      // 1. Primary: Fetch temporary time-limited credentials from /turn_credentials
      const res = await fetch(`/turn_credentials?user_id=${encodeURIComponent(uid)}`);
      if (res.ok) {
        const data = await res.json();
        if (data && data.uris && data.username && data.password) {
          const iceServers = [
            { urls: 'stun:stun.l.google.com:19302' },
            {
              urls: data.uris,
              username: data.username,
              credential: data.password
            }
          ];
          console.log("[Omnicast SDK] Configured TURN credentials with TCP/UDP fallback:", iceServers);
          this.rtcConfig = {
            iceServers: iceServers,
            iceTransportPolicy: 'all',
            iceCandidatePoolSize: 2
          };
          return this.rtcConfig;
        }
      }

      // 2. Fallback to /api/ice-servers
      const fallbackRes = await fetch('/api/ice-servers');
      if (fallbackRes.ok) {
        const fallbackData = await fallbackRes.json();
        if (fallbackData && fallbackData.iceServers) {
          console.log("[Omnicast SDK] Using ICE Servers from /api/ice-servers:", fallbackData.iceServers);
          this.rtcConfig = {
            iceServers: fallbackData.iceServers,
            iceTransportPolicy: 'all',
            iceCandidatePoolSize: 2
          };
          return this.rtcConfig;
        }
      }
    } catch (err) {
      console.warn("[Omnicast SDK] Could not fetch dynamic TURN credentials, using defaults:", err);
    }
    return this.rtcConfig;
  }

  /**
   * Publishes a local media stream (Host or Co-Host) to the server
   * @param {MediaStream} localStream - Camera & Microphone stream
   * @returns {Promise<RTCPeerConnection>}
   */
  async publishStream(localStream) {
    if (!localStream) {
      throw new Error('Local MediaStream must be provided to publishStream');
    }

    this.localStream = localStream;
    await this.fetchIceServers();
    this._initPeerConnection();

    // 1. Configure Simulcast Video Transceiver with 3 quality layers (High 'f', Med 'h', Low 'q')
    const videoTrack = localStream.getVideoTracks()[0];
    if (videoTrack) {
      this.videoTransceiver = this.peerConnection.addTransceiver(videoTrack, {
        direction: 'sendonly',
        streams: [localStream],
        sendEncodings: [
          { rid: 'f', active: true, maxBitrate: 1200000 },
          { rid: 'h', active: true, maxBitrate: 500000, scaleResolutionDownBy: 2 },
          { rid: 'q', active: true, maxBitrate: 150000, scaleResolutionDownBy: 4 }
        ]
      });
    }

    // Add Audio Track
    const audioTrack = localStream.getAudioTracks()[0];
    if (audioTrack) {
      this.peerConnection.addTransceiver(audioTrack, {
        direction: 'sendonly',
        streams: [localStream]
      });
    }

    // 2. এরপর Offer তৈরি করো:
    const offer = await this.peerConnection.createOffer();

    // 3. সবশেষে Local Description সেট করে সার্ভারে পাঠাও:
    await this.peerConnection.setLocalDescription(offer);

    // Send publish offer to signaling server
    this.sendSignalingMessage('publish', {
      sdp: offer.sdp,
      type: offer.type
    });

    return this.peerConnection;
  }

  /**
   * Subscribes to the live room media broadcast as a Viewer
   * @returns {Promise<RTCPeerConnection>}
   */
  async joinAsViewer() {
    await this.fetchIceServers();
    this._initPeerConnection();
    this.remoteStream = new MediaStream();

    // Add receive-only transceivers for audio & video
    this.peerConnection.addTransceiver('video', { direction: 'recvonly' });
    this.peerConnection.addTransceiver('audio', { direction: 'recvonly' });

    // Handle incoming remote media tracks (Audio and Video)
    this.peerConnection.ontrack = (event) => {
      console.log("Received remote track:", event.track.kind, "ID:", event.track.id);
      if (!this.remoteStream.getTracks().some(t => t.id === event.track.id)) {
        this.remoteStream.addTrack(event.track);
      }

      this.emit('onTrack', {
        track: event.track,
        stream: this.remoteStream,
        streams: event.streams,
        kind: event.track.kind
      });
    };

    // Create Viewer SDP Offer
    const offer = await this.peerConnection.createOffer();
    await this.peerConnection.setLocalDescription(offer);

    // Send join_room offer to signaling server
    this.sendSignalingMessage('join_room', {
      sdp: offer.sdp,
      type: offer.type
    });

    return this.peerConnection;
  }

  /**
   * Subscribes to a specific Co-Host's media stream
   * @param {string} cohostId - ID of the co-host
   */
  subscribeCoHost(cohostId) {
    if (!cohostId) return;
    this.sendSignalingMessage('subscribe_cohost', {
      cohost_id: cohostId
    });
  }

  /**
   * Initializes internal RTCPeerConnection and attaches standard ICE / state listeners
   * @private
   */
  _initPeerConnection() {
    if (this.peerConnection) {
      try {
        this.peerConnection.close();
      } catch (_) {}
    }

    this.peerConnection = new RTCPeerConnection(this.rtcConfig);

    // Handle local ICE candidates and relay them to server
    this.peerConnection.onicecandidate = (event) => {
      if (event.candidate && this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.sendSignalingMessage('ice', event.candidate.toJSON());
      }
    };

    // ICE Connection state change (Seamless WiFi-to-4G Network Switching)
    this.peerConnection.oniceconnectionstatechange = () => {
      const state = this.peerConnection.iceConnectionState;
      this.emit('onIceStateChange', state);
      if (state === 'disconnected' || state === 'failed') {
        console.warn(`[Omnicast SDK] ICE State is '${state}'. Network shift detected (WiFi/Cellular). Requesting ICE Restart...`);
        this.restartIce();
      }
    };

    // PeerConnection general state change
    this.peerConnection.onconnectionstatechange = () => {
      this.emit('onConnectionStateChange', this.peerConnection.connectionState);
    };
  }

  /**
   * Triggers an ICE Restart to renegotiate network paths seamlessly without tearing down media tracks
   */
  restartIce() {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      console.log('[Omnicast SDK] Dispatched ICE Restart request to SFU');
      this.sendSignalingMessage('ice_restart', {
        event: 'ice_restart',
        room_id: this.roomId,
        user_id: this.userId
      });
    }
  }

  /**
   * Dynamically toggles active status for a simulcast encoding layer on the Host transceiver (Dynacast)
   * @param {string} rid - Layer RID ('f', 'h', 'q')
   * @param {boolean} active - Active status
   */
  async _setLayerActive(rid, active) {
    if (!this.videoTransceiver || !this.videoTransceiver.sender) return;
    try {
      const params = this.videoTransceiver.sender.getParameters();
      if (!params || !params.encodings) return;
      let modified = false;
      params.encodings.forEach(enc => {
        if (enc.rid === rid && enc.active !== active) {
          enc.active = active;
          modified = true;
          console.log(`[Dynacast] Set encoding RID '${rid}' active: ${active}`);
        }
      });
      if (modified) {
        await this.videoTransceiver.sender.setParameters(params);
      }
    } catch (err) {
      console.warn(`[Dynacast Error] Failed to update layer ${rid} active status:`, err);
    }
  }

  // ==========================================================================
  // 3. INTERACTION HELPERS (Chat, Gifting, Seat & Main Stage)
  // ==========================================================================

  /**
   * Send a live chat message
   * @param {string} text - Message text
   */
  sendChatMessage(text) {
    if (!text) return;
    this.sendSignalingMessage('chat', { text: text });
  }

  /**
   * Send a gift with coins
   * @param {number} [coins=10] - Number of coins
   */
  sendGift(coins = 10) {
    this.sendSignalingMessage('gift', { coins: coins });
  }

  /**
   * Request a Co-Host seat (Viewer to Host)
   */
  requestSeat() {
    this.sendSignalingMessage('seat_request', {});
  }

  /**
   * Accept a Co-Host seat request (Host to Viewer)
   * @param {string} targetUserId - Viewer User ID
   */
  acceptSeat(targetUserId) {
    if (!targetUserId) return;
    this.sendSignalingMessage('seat_accept', { target_user: targetUserId }, targetUserId);
  }

  /**
   * Promote or pin a participant to the Main Seat
   * @param {string} targetUserId - Target User ID to feature on main stage
   */
  setMainSeat(targetUserId) {
    if (!targetUserId) return;
    this.sendSignalingMessage('set_main_seat', { target_id: targetUserId }, targetUserId);
  }

  /**
   * Broadcasts mute/unmute state changes for audio and video to the room
   * @param {boolean} mutedAudio
   * @param {boolean} mutedVideo
   */
  setMediaState(mutedAudio, mutedVideo) {
    this.sendSignalingMessage('media_state', {
      muted_audio: !!mutedAudio,
      muted_video: !!mutedVideo
    });
  }

  /**
   * Steps down from the Co-Host seat voluntarily
   */
  leaveSeat() {
    this.sendSignalingMessage('leave_seat', {});
  }

  /**
   * Host kicks a Co-Host from their seat
   * @param {string} targetUserId - The Co-Host's user ID to kick
   */
  kickSeat(targetUserId) {
    if (!targetUserId) return;
    this.sendSignalingMessage('kick_seat', { target_user: targetUserId }, targetUserId);
  }

  /**
   * Sends a PK battle request to an opponent room
   * @param {string} targetRoomId - Opponent Room ID
   */
  requestPK(targetRoomId) {
    if (!targetRoomId) return;
    this.sendSignalingMessage('pk_request', { target_room: targetRoomId }, targetRoomId);
  }

  /**
   * Accepts a PK battle request from an opponent room
   * @param {string} fromRoomId - Opponent Room ID
   */
  acceptPK(fromRoomId) {
    if (!fromRoomId) return;
    this.sendSignalingMessage('pk_accept', { target_room: fromRoomId }, fromRoomId);
  }

  /**
   * Ends the active PK battle session
   */
  stopPK() {
    this.sendSignalingMessage('pk_stop', {});
  }

  // ==========================================================================
  // 4. CLEANUP & DISPOSAL
  // ==========================================================================

  /**
   * Closes PeerConnection and local stream tracks without disconnecting WebSocket
   */
  stopMedia() {
    if (this.localStream) {
      this.localStream.getTracks().forEach(track => {
        try {
          track.stop();
        } catch (_) {}
      });
      this.localStream = null;
    }

    if (this.peerConnection) {
      try {
        this.peerConnection.close();
      } catch (_) {}
      this.peerConnection = null;
    }

    this.remoteStream = null;
  }

  /**
   * Disconnects WebSocket connection and stops media
   */
  disconnect() {
    this.stopMedia();

    if (this.ws) {
      try {
        this.ws.close();
      } catch (_) {}
      this.ws = null;
    }

    this.isConnected = false;
  }

  /**
   * Full teardown of the instance, listeners, and resources
   */
  dispose() {
    this.disconnect();
    this.removeAllListeners();
    this.isDisposed = true;
  }
}

// Support both ES Module and CommonJS / Browser global environments
if (typeof module !== 'undefined' && module.exports) {
  module.exports = { EventEmitter, LiveMediaCore };
} else if (typeof window !== 'undefined') {
  window.EventEmitter = EventEmitter;
  window.LiveMediaCore = LiveMediaCore;
}
