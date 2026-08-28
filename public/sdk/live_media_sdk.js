/**
 * ============================================================================
 * Live Media Server - Unified SDK Architecture
 * ============================================================================
 * Production-ready, zero-dependency WebRTC & Real-time Live Streaming SDK.
 * Fully compatible with Pion SFU Backend, Redis State Management, Simulcast/Dynacast,
 * Seat Management (Co-hosting), PK Battles, and Coturn TURN REST API.
 *
 * @version 1.0.0
 * @license MIT
 * ============================================================================
 */

(function (global, factory) {
  if (typeof exports === 'object' && typeof module !== 'undefined') {
    module.exports = factory();
  } else if (typeof define === 'function' && define.amd) {
    define(factory);
  } else {
    global = typeof globalThis !== 'undefined' ? globalThis : global || self;
    global.LiveMediaSDK = factory();
  }
})(this, function () {
  'use strict';

  // ==========================================================================
  // 0. EVENT EMITTER (LIGHTWEIGHT EVENT BUS)
  // ==========================================================================
  class EventEmitter {
    constructor() {
      this._events = {};
    }

    on(event, listener) {
      if (typeof listener !== 'function') return this;
      if (!this._events[event]) this._events[event] = [];
      this._events[event].push(listener);
      return this;
    }

    once(event, listener) {
      if (typeof listener !== 'function') return this;
      const onceWrapper = (...args) => {
        this.off(event, onceWrapper);
        listener.apply(this, args);
      };
      onceWrapper._originalListener = listener;
      return this.on(event, onceWrapper);
    }

    off(event, listener) {
      if (!this._events[event]) return this;
      if (!listener) {
        delete this._events[event];
        return this;
      }
      this._events[event] = this._events[event].filter(
        l => l !== listener && l._originalListener !== listener
      );
      return this;
    }

    emit(event, ...args) {
      if (!this._events[event]) return false;
      const listeners = [...this._events[event]];
      for (const listener of listeners) {
        try {
          listener.apply(this, args);
        } catch (err) {
          console.error(`[LiveMediaSDK EventEmitter Error] Event '${event}':`, err);
        }
      }
      return true;
    }

    removeAllListeners() {
      this._events = {};
    }
  }

  // ==========================================================================
  // 1. LIVE STATE MANAGER (REACTIVE IN-MEMORY CACHE & UI SYNCHRONIZER)
  // ==========================================================================
  class LiveStateManager extends EventEmitter {
    constructor() {
      super();
      this.reset();
    }

    reset() {
      this.roomId = '';
      this.roomName = '';
      this.hostId = '';
      this.mainSeatId = '';
      this.totalViewers = 0;
      this.viewersList = [];
      this.hostScore = 0;
      this.activeSeats = {}; // seatId -> userId
      this.mediaStates = {}; // userId -> { muted_audio: bool, muted_video: bool }
      this.pkSession = null; // { sessionId, room1, room2, host1, host2, score1, score2 }
    }

    /**
     * Applies an authoritative RoomState snapshot from Late Joiner Sync (room_info_sync)
     * @param {Object} state - Authoritative state payload from Redis/Backend
     */
    syncRoomInfo(state) {
      if (!state) return;
      this.roomId = state.room_id || this.roomId;
      this.roomName = state.room_name || this.roomName;
      this.hostId = state.host_id || this.hostId;
      this.mainSeatId = state.main_seat_id || this.hostId;
      this.totalViewers = typeof state.total_viewers === 'number' ? state.total_viewers : this.totalViewers;
      this.hostScore = typeof state.host_score === 'number' ? state.host_score : this.hostScore;
      this.activeSeats = state.active_seats ? { ...state.active_seats } : {};
      this.mediaStates = state.media_states ? { ...state.media_states } : {};

      this.emit('onStateSynced', this.getSnapshot());
      this.emit('onViewersUpdated', { totalViewers: this.totalViewers, viewersList: this.viewersList });
      this.emit('onScoreUpdated', this.hostScore);
      this.emit('onSeatsUpdated', this.activeSeats);
    }

    updateViewers(total, list) {
      this.totalViewers = total;
      if (Array.isArray(list)) this.viewersList = list;
      this.emit('onViewersUpdated', { totalViewers: this.totalViewers, viewersList: this.viewersList });
    }

    updateScore(score) {
      this.hostScore = score;
      this.emit('onScoreUpdated', this.hostScore);
    }

    updateSeat(seatId, userId) {
      if (!userId) {
        delete this.activeSeats[seatId];
      } else {
        this.activeSeats[seatId] = userId;
      }
      this.emit('onSeatsUpdated', { ...this.activeSeats });
    }

    updateMediaState(userId, mediaState) {
      this.mediaStates[userId] = {
        muted_audio: !!mediaState.muted_audio,
        muted_video: !!mediaState.muted_video
      };
      this.emit('onMediaStateChanged', { userId, mediaState: this.mediaStates[userId] });
    }

    setPKSession(session) {
      this.pkSession = session;
      this.emit('onPKSessionChanged', this.pkSession);
    }

    updatePKScore(score1, score2) {
      if (this.pkSession) {
        this.pkSession.score1 = score1;
        this.pkSession.score2 = score2;
      }
      this.emit('onPKScoreUpdated', { score1, score2 });
    }

    getSnapshot() {
      return {
        roomId: this.roomId,
        roomName: this.roomName,
        hostId: this.hostId,
        mainSeatId: this.mainSeatId,
        totalViewers: this.totalViewers,
        viewersList: [...this.viewersList],
        hostScore: this.hostScore,
        activeSeats: { ...this.activeSeats },
        mediaStates: { ...this.mediaStates },
        pkSession: this.pkSession ? { ...this.pkSession } : null
      };
    }
  }

  // ==========================================================================
  // 2. LIVE MEDIA MANAGER (WEBRTC SFU, SIMULCAST, ABR & DYNACAST)
  // ==========================================================================
  class LiveMediaManager extends EventEmitter {
    constructor(roomClient) {
      super();
      this.roomClient = roomClient;

      /** @type {RTCPeerConnection|null} */
      this.peerConnection = null;
      /** @type {MediaStream|null} */
      this.localStream = null;
      /** @type {MediaStream|null} */
      this.remoteStream = null;
      /** @type {RTCRtpTransceiver|null} */
      this.videoTransceiver = null;

      this.coHostStreams = new Map(); // coHostId -> MediaStream
      this.isPublishing = false;
      this.isAudioMuted = false;
      this.isVideoMuted = false;
    }

    /**
     * Initializes a new RTCPeerConnection with dynamic Coturn ICE servers
     * @private
     */
    _createPeerConnection() {
      if (this.peerConnection) {
        try { this.peerConnection.close(); } catch (_) {}
      }

      const config = this.roomClient.iceConfig || {
        iceServers: [
          { urls: 'stun:stun.l.google.com:19302' },
          { urls: 'stun:stun1.l.google.com:19302' }
        ]
      };

      this.peerConnection = new RTCPeerConnection(config);

      this.peerConnection.onicecandidate = (event) => {
        if (event.candidate) {
          this.roomClient.send('ice', event.candidate.toJSON());
        }
      };

      this.peerConnection.oniceconnectionstatechange = () => {
        this.emit('onIceStateChange', this.peerConnection.iceConnectionState);
      };

      this.peerConnection.onconnectionstatechange = () => {
        this.emit('onConnectionStateChange', this.peerConnection.connectionState);
      };

      this.peerConnection.ontrack = (event) => {
        this._handleRemoteTrack(event);
      };

      return this.peerConnection;
    }

    /**
     * Handles incoming remote media tracks (Host, Co-Hosts, PK opponent)
     * @private
     */
    _handleRemoteTrack(event) {
      if (!this.remoteStream) {
        this.remoteStream = new MediaStream();
      }
      this.remoteStream.addTrack(event.track);
      this.emit('onRemoteTrack', {
        track: event.track,
        stream: event.streams[0] || this.remoteStream,
        kind: event.track.kind
      });
    }

    /**
     * Publishes local camera & microphone with 3 Simulcast Layers (High 'f', Med 'h', Low 'q')
     * @param {Object} [options] - Stream configuration
     * @param {MediaStream} [options.stream] - Pre-acquired MediaStream (optional)
     * @param {boolean} [options.video=true] - Enable camera
     * @param {boolean} [options.audio=true] - Enable microphone
     * @param {boolean} [options.simulcast=true] - Enable 3-layer simulcast
     * @returns {Promise<MediaStream>}
     */
    async publishCamera(options = {}) {
      const {
        stream = null,
        video = true,
        audio = true,
        simulcast = true
      } = options;

      if (stream) {
        this.localStream = stream;
      } else {
        const videoConstraints = typeof video === 'object' ? video : (video ? {
          width: { ideal: 640, max: 640 },
          height: { ideal: 480, max: 480 },
          frameRate: { ideal: 24, max: 30 }
        } : false);

        this.localStream = await navigator.mediaDevices.getUserMedia({ video: videoConstraints, audio });
      }

      this._createPeerConnection();

      // 1. Configure Simulcast Video Transceiver (High 'f', Med 'h', Low 'q')
      const videoTrack = this.localStream.getVideoTracks()[0];
      if (videoTrack) {
        const encodings = simulcast ? [
          { rid: 'f', active: true, maxBitrate: 1200000 },
          { rid: 'h', active: true, maxBitrate: 500000, scaleResolutionDownBy: 2 },
          { rid: 'q', active: true, maxBitrate: 150000, scaleResolutionDownBy: 4 }
        ] : [{ active: true, maxBitrate: 1200000 }];

        this.videoTransceiver = this.peerConnection.addTransceiver(videoTrack, {
          direction: 'sendonly',
          streams: [this.localStream],
          sendEncodings: encodings
        });
        this._forceVP8(this.videoTransceiver);
      }

      // Add Audio Transceiver
      const audioTrack = this.localStream.getAudioTracks()[0];
      if (audioTrack) {
        this.peerConnection.addTransceiver(audioTrack, {
          direction: 'sendonly',
          streams: [this.localStream]
        });
      }

      // 2. Create SDP Offer after transceivers are attached
      const offer = await this.peerConnection.createOffer();
      await this.peerConnection.setLocalDescription(offer);

      // 3. Send publish offer over WebSocket
      this.roomClient.send('publish', {
        sdp: offer.sdp,
        type: offer.type
      });

      this.isPublishing = true;
      this.emit('onLocalStream', this.localStream);
      return this.localStream;
    }

    _forceVP8(transceiver) {
      if (transceiver && 'setCodecPreferences' in transceiver && typeof RTCRtpReceiver !== 'undefined' && RTCRtpReceiver.getCapabilities) {
        try {
          const capabilities = RTCRtpReceiver.getCapabilities('video');
          if (capabilities && capabilities.codecs) {
            const vp8Codecs = capabilities.codecs.filter(c => c.mimeType.toLowerCase() === 'video/vp8');
            const otherCodecs = capabilities.codecs.filter(c => c.mimeType.toLowerCase() !== 'video/vp8' && c.mimeType.toLowerCase() !== 'video/h264');
            if (vp8Codecs.length > 0) {
              transceiver.setCodecPreferences([...vp8Codecs, ...otherCodecs]);
            }
          }
        } catch (_) {}
      }
    }

    /**
     * Subscribes to live room broadcast as a Viewer
     */
    async subscribeViewer() {
      this._createPeerConnection();
      this.remoteStream = new MediaStream();

      this.peerConnection.addTransceiver('video', { direction: 'recvonly' });
      this.peerConnection.addTransceiver('audio', { direction: 'recvonly' });

      // Signal join to server
      this.roomClient.send('join_room', {
        room_id: this.roomClient.roomId,
        user_id: this.roomClient.userId,
        user_name: this.roomClient.userName,
        role: 'viewer'
      });
    }

    /**
     * Sets remote SDP Answer received from server
     * @param {RTCSessionDescriptionInit} sdpInit
     */
    async handleAnswer(sdpInit) {
      if (!this.peerConnection) return;
      await this.peerConnection.setRemoteDescription(new RTCSessionDescription(sdpInit));
    }

    /**
     * Handles server renegotiation offer (e.g. Co-Host joined or PK started)
     * @param {RTCSessionDescriptionInit} offerInit
     */
    async handleRenegotiationOffer(offerInit) {
      if (!this.peerConnection) return;
      await this.peerConnection.setRemoteDescription(new RTCSessionDescription(offerInit));
      const answer = await this.peerConnection.createAnswer();
      await this.peerConnection.setLocalDescription(answer);

      this.roomClient.send('answer', {
        sdp: answer.sdp,
        type: answer.type
      });
    }

    /**
     * Adds remote ICE candidate received from server
     * @param {RTCIceCandidateInit} candidateInit
     */
    async handleRemoteCandidate(candidateInit) {
      if (!this.peerConnection) return;
      try {
        await this.peerConnection.addIceCandidate(new RTCIceCandidate(candidateInit));
      } catch (err) {
        console.warn('[LiveMediaSDK ICE Candidate Error]:', err);
      }
    }

    /**
     * Toggles microphone mute status and synchronizes to server
     * @param {boolean} [mute]
     */
    toggleAudio(mute) {
      if (!this.localStream) return;
      const audioTrack = this.localStream.getAudioTracks()[0];
      if (audioTrack) {
        this.isAudioMuted = typeof mute === 'boolean' ? mute : !this.isAudioMuted;
        audioTrack.enabled = !this.isAudioMuted;

        this.roomClient.send('media_state', {
          muted_audio: this.isAudioMuted,
          muted_video: this.isVideoMuted
        });
      }
    }

    /**
     * Toggles camera video mute status and synchronizes to server
     * @param {boolean} [mute]
     */
    toggleVideo(mute) {
      if (!this.localStream) return;
      const videoTrack = this.localStream.getVideoTracks()[0];
      if (videoTrack) {
        this.isVideoMuted = typeof mute === 'boolean' ? mute : !this.isVideoMuted;
        videoTrack.enabled = !this.isVideoMuted;

        this.roomClient.send('media_state', {
          muted_audio: this.isAudioMuted,
          muted_video: this.isVideoMuted
        });
      }
    }

    /**
     * Dynacast Encoding Control: sets encoding active state on Host transceiver
     * @param {string} rid - Layer RID ('f', 'h', 'q')
     * @param {boolean} active - Active state
     */
    async setDynacastLayerActive(rid, active) {
      if (!this.videoTransceiver || !this.videoTransceiver.sender) return;
      try {
        const params = this.videoTransceiver.sender.getParameters();
        if (!params || !params.encodings) return;
        let changed = false;
        params.encodings.forEach(enc => {
          if (enc.rid === rid && enc.active !== active) {
            enc.active = active;
            changed = true;
          }
        });
        if (changed) {
          await this.videoTransceiver.sender.setParameters(params);
          console.log(`[LiveMediaSDK Dynacast] Set encoding '${rid}' active: ${active}`);
        }
      } catch (err) {
        console.warn(`[LiveMediaSDK Dynacast Error]:`, err);
      }
    }

    stop() {
      if (this.localStream) {
        this.localStream.getTracks().forEach(t => t.stop());
        this.localStream = null;
      }
      if (this.peerConnection) {
        try { this.peerConnection.close(); } catch (_) {}
        this.peerConnection = null;
      }
      this.isPublishing = false;
    }
  }

  // ==========================================================================
  // 3. LIVE ROOM CLIENT (AUTHENTICATION, WEBSOCKET & SIGNALING ROUTING)
  // ==========================================================================
  class LiveRoomClient extends EventEmitter {
    constructor(options = {}) {
      super();
      this.hostUrl = options.hostUrl || 'http://localhost:8080';
      this.wsUrl = options.wsUrl || '';
      this.apiKey = options.apiKey || '';
      this.apiSecret = options.apiSecret || '';
      this.userId = options.userId || 'usr_' + Math.random().toString(36).substring(2, 9);
      this.userName = options.userName || 'User';
      this.avatarUrl = options.avatarUrl || '';
      this.token = options.token || '';
      this.role = options.role || 'viewer';
      this.roomId = options.roomId || '';
      this.roomName = options.roomName || '';

      this.iceConfig = null;
      /** @type {WebSocket|null} */
      this.ws = null;
      this.isConnected = false;
      this.reconnectAttempts = 0;
      this.maxReconnectAttempts = 10;
    }

    /**
     * Authenticates with the REST API /api/auth/token and fetches JWT + Coturn ICE servers
     */
    async authenticate() {
      const authEndpoint = `${this.hostUrl.replace(/\/$/, '')}/api/auth/token`;
      const response = await fetch(authEndpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          api_key: this.apiKey,
          api_secret: this.apiSecret,
          user_id: this.userId,
          user_name: this.userName,
          avatar_url: this.avatarUrl
        })
      });

      if (!response.ok) {
        throw new Error(`Authentication failed with status ${response.status}: ${await response.text()}`);
      }

      const data = await response.json();
      if (data.status !== 'success' || !data.token) {
        throw new Error(`Authentication error: ${data.error || 'Unknown response'}`);
      }

      this.token = data.token;
      if (data.ice_servers && Array.isArray(data.ice_servers)) {
        this.iceConfig = { iceServers: data.ice_servers };
      }
      return data;
    }

    /**
     * Establishes authenticated WebSocket connection
     */
    async connectWS() {
      return new Promise((resolve, reject) => {
        let wsTarget = this.wsUrl;
        if (!wsTarget) {
          const wsProtocol = this.hostUrl.startsWith('https') ? 'wss://' : 'ws://';
          const hostOnly = this.hostUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');
          wsTarget = `${wsProtocol}${hostOnly}/ws`;
        }

        const separator = wsTarget.includes('?') ? '&' : '?';
        const urlWithToken = `${wsTarget}${separator}token=${encodeURIComponent(this.token)}`;

        this.ws = new WebSocket(urlWithToken);

        this.ws.onopen = () => {
          this.isConnected = true;
          this.reconnectAttempts = 0;
          this.emit('onConnected');
          resolve();
        };

        this.ws.onclose = (event) => {
          this.isConnected = false;
          this.emit('onDisconnected', event);
          this._handleAutoReconnect();
        };

        this.ws.onerror = (err) => {
          this.emit('onError', err);
          if (!this.isConnected) reject(err);
        };

        this.ws.onmessage = (event) => {
          this._handleMessage(event.data);
        };
      });
    }

    _handleAutoReconnect() {
      if (this.reconnectAttempts >= this.maxReconnectAttempts) return;
      this.reconnectAttempts++;
      const backoff = Math.min(1000 * Math.pow(1.5, this.reconnectAttempts), 10000);
      setTimeout(() => {
        if (!this.isConnected) {
          this.connectWS().catch(() => {});
        }
      }, backoff);
    }

    _handleMessage(rawData) {
      try {
        const msg = JSON.parse(rawData);
        const action = msg.action || msg.event;
        const payload = msg.payload || msg.data;

        this.emit('onMessage', { action, payload, raw: msg });
        this.emit(action, { payload, raw: msg });
      } catch (err) {
        console.warn('[LiveMediaSDK WS Message Parse Error]:', err);
      }
    }

    /**
     * Transmits a WebSocket envelope matching our backend WSMessage structure
     * @param {string} action - Event/Action name
     * @param {Object} payload - Data payload
     */
    send(action, payload = {}) {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
        console.warn(`[LiveMediaSDK WS] Cannot send '${action}': WebSocket not open`);
        return false;
      }

      const msg = {
        action: action,
        event: action,
        room_id: this.roomId,
        room_name: this.roomName,
        user_id: this.userId,
        payload: payload
      };

      this.ws.send(JSON.stringify(msg));
      return true;
    }

    // Chat, Gifting & Seat Requests
    sendChat(text) {
      return this.send('chat', {
        user_id: this.userId,
        user_name: this.userName,
        message: text,
        avatar_url: this.avatarUrl
      });
    }

    sendGift(giftName, coins = 10) {
      return this.send('gift', {
        gift_name: giftName,
        coins: Number(coins),
        user_id: this.userId,
        user_name: this.userName
      });
    }

    requestSeat(seatId = 1) {
      return this.send('seat_request', {
        seat_id: String(seatId),
        user_id: this.userId,
        user_name: this.userName
      });
    }

    acceptSeat(viewerId, seatId = 1) {
      return this.send('seat_accept', {
        seat_id: String(seatId),
        target_user: viewerId
      });
    }

    leaveSeat() {
      return this.send('leave_seat', { user_id: this.userId });
    }

    kickSeat(targetUserId) {
      return this.send('kick_seat', { target_user: targetUserId });
    }

    requestPK(targetRoomId) {
      return this.send('pk_request', { target_room_id: targetRoomId });
    }

    acceptPK(fromRoomId) {
      return this.send('pk_accept', { target_room_id: fromRoomId });
    }

    stopPK(targetRoomId) {
      return this.send('pk_stop', { target_room_id: targetRoomId });
    }

    disconnect() {
      if (this.ws) {
        this.ws.close();
        this.ws = null;
      }
      this.isConnected = false;
    }
  }

  // ==========================================================================
  // 4. UNIFIED SDK FACADE (TOP-LEVEL DEVELOPER API)
  // ==========================================================================
  class LiveMediaSDK extends EventEmitter {
    constructor() {
      super();
      this.room = new LiveRoomClient();
      this.media = new LiveMediaManager(this.room);
      this.state = new LiveStateManager();

      this._wireEvents();
    }

    _wireEvents() {
      // Route WebSocket events to Media & State managers
      this.room.on('onMessage', ({ action, payload, raw }) => {
        switch (action) {
          // Late Joiner Sync / Room Snapshot
          case 'room_info_sync':
          case 'room_state':
          case 'room_info':
            this.state.syncRoomInfo(payload || raw.data);
            break;

          case 'viewer_update':
            this.state.updateViewers(raw.total_viewers || (payload ? payload.total_viewers : 0), raw.viewers_list || []);
            break;

          case 'gift_processed':
          case 'score_updated':
            if (payload && typeof payload.new_score === 'number') {
              this.state.updateScore(payload.new_score);
            }
            break;

          case 'seat_updated':
            if (payload) {
              this.state.updateSeat(payload.seat_id, payload.user_id);
            }
            break;

          case 'media_state_updated':
            if (payload && payload.user_id) {
              this.state.updateMediaState(payload.user_id, payload);
            }
            break;

          case 'pk_started':
            this.state.setPKSession(payload);
            break;

          case 'pk_score_update':
            if (payload) {
              this.state.updatePKScore(payload.room_a_score || payload.score_1, payload.room_b_score || payload.score_2);
            }
            break;

          case 'pk_ended':
            this.state.setPKSession(null);
            break;

          // WebRTC Signaling Exchanges
          case 'answer':
            this.media.handleAnswer(payload);
            break;

          case 'offer':
          case 'renegotiate':
            this.media.handleRenegotiationOffer(payload);
            break;

          case 'ice':
          case 'candidate':
            this.media.handleRemoteCandidate(payload);
            break;

          // Dynacast Layer Optimization
          case 'dynacast_pause_layer':
            if (payload && payload.layer) {
              this.media.setDynacastLayerActive(payload.layer, false);
            }
            break;

          case 'dynacast_resume_layer':
            if (payload && payload.layer) {
              this.media.setDynacastLayerActive(payload.layer, true);
            }
            break;
        }

        // Forward raw event to SDK listeners
        this.emit(action, payload || raw);
      });
    }

    /**
     * Initializes SDK credentials, authenticates with REST API, and connects signaling
     * @param {Object} config - Initialization options
     * @param {string} config.hostUrl - Base URL of the Media Server (e.g., http://localhost:8080)
     * @param {string} config.apiKey - API Key configured in .env
     * @param {string} config.apiSecret - API Secret configured in .env
     * @param {string} [config.userId] - User ID
     * @param {string} [config.userName] - Display name
     * @param {string} [config.avatarUrl] - User avatar image URL
     */
    async initialize(config = {}) {
      this.room.hostUrl = config.hostUrl || this.room.hostUrl;
      this.room.apiKey = config.apiKey || this.room.apiKey;
      this.room.apiSecret = config.apiSecret || this.room.apiSecret;
      this.room.userId = config.userId || this.room.userId;
      this.room.userName = config.userName || this.room.userName;
      this.room.avatarUrl = config.avatarUrl || this.room.avatarUrl;

      // 1. REST Authentication & Coturn ICE generation
      await this.room.authenticate();

      // 2. Connect WebSocket
      await this.room.connectWS();
      return this;
    }

    /**
     * Creates or hosts a live room
     * @param {Object} options
     * @param {string} options.roomId - Room identifier
     * @param {string} [options.roomName] - Room title
     */
    async createRoom(options = {}) {
      this.room.roomId = options.roomId || 'room-' + Date.now();
      this.room.roomName = options.roomName || this.room.roomId;
      this.room.role = 'host';
      this.state.roomId = this.room.roomId;
      this.state.hostId = this.room.userId;
    }

    /**
     * Joins an existing live room as a viewer
     * @param {string} roomId
     */
    async joinRoom(roomId) {
      this.room.roomId = roomId;
      this.room.role = 'viewer';
      this.state.roomId = roomId;
      await this.media.subscribeViewer();
    }

    destroy() {
      this.media.stop();
      this.room.disconnect();
      this.state.reset();
      this.removeAllListeners();
    }
  }

  return LiveMediaSDK;
});
