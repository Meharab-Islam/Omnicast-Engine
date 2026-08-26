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

    this.rtcConfig = options.rtcConfig || {
      iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        { urls: 'stun:stun1.l.google.com:19302' }
      ]
    };

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

      // 4. Gift Received
      case 'gift_received':
        this.emit('onGiftReceived', {
          senderId: payload ? payload.sender_id : msg.user_id,
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

      // 6. Late Joiner Room Info Sync
      case 'room_info':
        if (payload) {
          this.emit('onRoomInfo', {
            roomId: payload.room_id || msg.room_id,
            hostId: payload.host_id,
            hostScore: payload.host_score || 0,
            mainSeatId: payload.main_seat_id || payload.host_id,
            activeCohosts: payload.active_cohosts || [],
            viewerCount: payload.viewer_count || 0,
            createdAt: payload.created_at,
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
          raw: msg
        });
        break;

      // 11. Room Closed by Host
      case 'room_closed':
        this.emit('onRoomClosed', {
          roomId: msg.room_id,
          raw: msg
        });
        break;

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
   * Fetches dynamic STUN/TURN ICE servers configuration from the server
   * @returns {Promise<RTCConfiguration>}
   */
  async fetchIceServers() {
    try {
      const res = await fetch('/api/ice-servers');
      if (res.ok) {
        const data = await res.json();
        if (data && data.iceServers) {
          console.log("Using ICE Servers:", data.iceServers);
          this.rtcConfig = { iceServers: data.iceServers };
          return this.rtcConfig;
        }
      }
    } catch (err) {
      console.warn("Could not fetch /api/ice-servers, using default:", err);
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

    // 1. অফার তৈরির আগেই Track অ্যাড করো (খুবই জরুরি):
    localStream.getTracks().forEach(track => {
      this.peerConnection.addTrack(track, localStream);
    });

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

    // ICE Connection state change
    this.peerConnection.oniceconnectionstatechange = () => {
      this.emit('onIceStateChange', this.peerConnection.iceConnectionState);
    };

    // PeerConnection general state change
    this.peerConnection.onconnectionstatechange = () => {
      this.emit('onConnectionStateChange', this.peerConnection.connectionState);
    };
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
