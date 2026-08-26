# 🚀 Go Live Media Server (SFU & Interactive Streaming)

A high-performance, low-latency **WebRTC SFU (Selective Forwarding Unit)** live streaming media server built in **Go (Pion WebRTC + Fiber v2)**. Designed for massive scalability, interactive live streaming, multi-guest co-hosting, live PK battles, and distributed edge cascading.

---

## 📑 সূচিপত্র (Table of Contents)
1. [🌟 প্রজেক্ট পরিচিতি ও কী কী ফিচার আছে (What's Built)](#-প্রজেক্ট-পরিচিতি-ও-কী-কী-ফিচার-আছে-whats-built)
2. [🏗️ আর্কিটেকচার ওভারভিউ (Architecture Overview)](#️-আর্কিটেকচার-ওভারভিউ-architecture-overview)
3. [🔌 ফ্রন্টএন্ড/UI-এর সাথে কানেক্ট করার নিয়ম (Frontend Connection Guide)](#-ফ্রন্টএন্ডui-এর-সাথে-কানেক্ট-করার-নিয়ম-frontend-connection-guide)
4. [📡 সিগন্যালিং প্রোটোকল ও মেসেজ ফরম্যাট (Signaling Protocol Spec)](#-সিগন্যালিং-প্রোটোকল-ও-মেসেজ-ফরম্যাট-signaling-protocol-spec)
5. [🛠️ নতুন ভাষার জন্য Client SDK বানানোর পূর্ণাঙ্গ গাইড (Build SDK in Any Language)](#️-নতুন-ভাষার-জন্য-client-sdk-বানানোর-পূর্ণাঙ্গ-গাইড-build-sdk-in-any-language)
   - [📱 Flutter (Dart) SDK](#1-flutter-dart-sdk-implementation)
   - [⚛️ React Native / TypeScript SDK](#2-react-native--typescript-sdk-implementation)
   - [🍏 iOS (Swift) SDK](#3-ios-swift-sdk-implementation)
   - [🤖 Android (Kotlin) SDK](#4-android-kotlin-sdk-implementation)
6. [🌐 REST API Endpoints](#-rest-api-endpoints)
7. [⚡ সার্ভার রান এবং ডেপ্লয়মেন্ট গাইড (Running & Deployment)](#-সার্ভার-রান-এবং-ডেপ্লয়মেন্ট-গাইড-running--deployment)
8. [🧪 টেস্টিং ও ভ্যালিডেশন (Testing)](#-টেস্টিং-ও-ভ্যালিডেশন-testing)

---

## 🌟 প্রজেক্ট পরিচিতি ও কী কী ফিচার আছে (What's Built)

এই প্রজেক্টটি আধুনিক লাইভ স্ট্রিমিং অ্যাপ্লিকেশনের (যেমন: TikTok Live, Bigo Live, Tango Live) মতো ফিচারসমৃদ্ধ একটি কমপ্লিট ব্যাকএন্ড ও রিয়েলটাইম মিডিয়া সার্ভার।

### 🔥 মূল ফিচারসমূহ:
- **WebRTC SFU Engine (Pion WebRTC):** VP8, H.264 ভিডিও কোডেক এবং Opus অডিও কোডেক সহ আল্ট্রা-লো লেটেন্সি (<500ms) লাইভ স্ট্রিমিং।
- **Simulcast & Dynamic Layer Switching:** ব্যান্ডউইথ অনুযায়ী স্বয়ংক্রিয়ভাবে ভিডিও রেজোলিউশন (High / Medium / Low) সুইচিং।
- **Multi-Guest / Co-Host Seating System:** হোস্ট যেকোনো ভিউয়ারকে কো-হোস্ট সিটে ইনভাইট বা অ্যাকসেপ্ট করতে পারে। সাথে সাথে সব ক্লায়েন্টে অটোমেটিক WebRTC Renegotiation হয়ে মাল্টিপল ভিডিও স্ট্রিম রেন্ডার হয়।
- **Main Stage / Seat Promotion:** যেকোনো কো-হোস্টকে এক ক্লিকে মেইন স্টেজে পিন করার ডায়নামিক সুইচিং।
- **Live PK Battle Engine:** দুটি আলাদা রুমের হোস্টদের মধ্যে রিয়েল-টাইম মিডিয়া ক্রস-রাউটিং (Room 1 হোস্টের স্ট্রিম Room 2 ভিউয়ারদের কাছে এবং Room 2 হোস্টের স্ট্রিম Room 1 ভিউয়ারদের কাছে পৌঁছে দেয়)।
- **SFU Cascading (Origin-to-Edge Replication):** হাজার হাজার ভিউয়ারের লোড হ্যান্ডেল করার জন্য Origin সার্ভার থেকে Edge সার্ভারে ইন্টার-সার্ভার WebRTC ট্র্যাক রিলে।
- **Distributed Redis Broker (Pub/Sub):** মাল্টি-নোড ক্লাস্টার সাপোর্ট, রুম মেটাডাটা সিঙ্ক, ক্রস-নোড চ্যাট ও গিফট ব্রডকাস্ট।
- **Real-Time Webhook Engine:** HMAC SHA-256 সিগনেচার সিকিউরিটি সহ `room_started`, `room_ended`, `user_joined`, `user_left`, `gift_sent` ইত্যাদি ইভেন্ট ব্যাকগ্রাউন্ড ওয়ার্কারের মাধ্যমে আপনার মূল বিজনেস API-তে পুশ করে।
- **30s Grace Period Reconnection:** হোস্টের সাময়িক নেটওয়ার্ক ডিসকানেক্ট হলে রুম তৎক্ষণাৎ বন্ধ না হয়ে ৩০ সেকেন্ড গ্রেস পিরিয়ডে থাকে এবং পুনরায় কানেক্ট হলে স্ট্রিম রিজউম হয়।
- **JWT Authentication & Security:** সুরক্ষিত WebSocket কানেকশন ও ইন্টার-সার্ভার বাইপাস সিক্রেট টোকেন।
- **Modern Ready-to-use Web UI & Headless SDK:** ডার্ক মোড, গ্লাস মরফিজম সহ প্রফেশনাল কন্ট্রোল প্যানেল এবং সম্পূর্ণ ফ্রেমওয়ার্ক-ইন্ডিপেন্ডেন্ট জাভাস্ক্রিপ্ট SDK (`public/sdk/core.js`)।

---

## 🏗️ আর্কিটেকচার ওভারভিউ (Architecture Overview)

```
                       +-----------------------+
                       |   REST API & Webhooks |
                       | (Auth, Rooms, Events) |
                       +-----------+-----------+
                                   |
    +------------------------------+------------------------------+
    |                                                             |
+---+----------------------+                       +--------------+-------+
|     Origin Node (SFU)    |  <-- Redis PubSub --> |       Edge Node       |
|  - Host Ingestion (RTP)  |  --- WebRTC Cascade-> | - Viewer Subscription |
|  - Simulcast Router      |                       | - Local Fanout (RTP)  |
+---+----------------------+                       +--------------+-------+
    |                                                             |
+---+-------------------------------------------------------------+-------+
|                       Client Applications                               |
|        [ Web UI ]       [ Flutter App ]       [ React Native / iOS ]     |
+-------------------------------------------------------------------------+
```

---

## 🔌 ফ্রন্টএন্ড/UI-এর সাথে কানেক্ট করার নিয়ম (Frontend Connection Guide)

যেকোনো ফ্রন্টএন্ড (React, Vue, Flutter, Vanilla JS ইত্যাদি) থেকে মিডিয়া সার্ভারের সাথে যুক্ত হওয়ার ৪টি ধাপ রয়েছে:

### ধাপ ১: টোকেন এবং ICE সার্ভার কনফিগারেশন আনা
```javascript
// ১. ICE সার্ভার কনফিগারেশন আনুন
const iceRes = await fetch("http://localhost:8080/api/ice-servers");
const { iceServers } = await iceRes.json();

// ২. অথেনটিকেশন টোকেন আনুন (প্রোডাকশনে আপনার নিজস্ব লগইন ব্যাকএন্ড থেকে আনবেন)
const authRes = await fetch("http://localhost:8080/auth/demo-token?user_id=user123&role=host&room_id=room101");
const { token } = await authRes.json();
```

### ধাপ ২: WebSocket কানেকশন স্থাপন
```javascript
const ws = new WebSocket(`ws://localhost:8080/ws?token=${encodeURIComponent(token)}`);

ws.onopen = () => console.log("Connected to Signaling Server!");
ws.onmessage = (event) => handleSignalingMessage(JSON.parse(event.data));
```

### ধাপ ৩: হোস্ট হিসেবে ব্রডকাস্ট শুরু করা (Publishing)
1. ক্যামেরা এবং মাইক্রোফোনের স্ট্রিম নিন:
   ```javascript
   const localStream = await navigator.mediaDevices.getUserMedia({ video: true, audio: true });
   ```
2. `RTCPeerConnection` তৈরি করুন এবং ট্র্যাক যোগ করুন:
   ```javascript
   const pc = new RTCPeerConnection({ iceServers });
   localStream.getTracks().forEach(track => pc.addTrack(track, localStream));
   ```
3. SDP Offer তৈরি করে সার্ভারে `publish` ইভেন্টে পাঠান:
   ```javascript
   const offer = await pc.createOffer();
   await pc.setLocalDescription(offer);

   ws.send(JSON.stringify({
     event: "publish",
     room_id: "room101",
     user_id: "user123",
     payload: { sdp: offer.sdp, type: offer.type }
   }));
   ```
4. সার্ভার থেকে `answer` ও `ice` আসলে সেট করুন:
   ```javascript
   // সার্ভার মেসেজ হ্যান্ডলার
   if (msg.event === "answer") {
     await pc.setRemoteDescription(new RTCSessionDescription(msg.payload));
   } else if (msg.event === "ice") {
     await pc.addIceCandidate(new RTCIceCandidate(msg.payload));
   }
   ```

### ধাপ ৪: ভিউয়ার হিসেবে লাইভ দেখা (Subscribing)
1. `RTCPeerConnection` তৈরি করে `recvonly` ট্রান্সসিভার অ্যাড করুন:
   ```javascript
   const pc = new RTCPeerConnection({ iceServers });
   pc.addTransceiver('video', { direction: 'recvonly' });
   pc.addTransceiver('audio', { direction: 'recvonly' });

   pc.ontrack = (event) => {
     document.getElementById("remoteVideo").srcObject = event.streams[0] || new MediaStream([event.track]);
   };
   ```
2. SDP Offer তৈরি করে `join_room` ইভেন্টে পাঠান:
   ```javascript
   const offer = await pc.createOffer();
   await pc.setLocalDescription(offer);

   ws.send(JSON.stringify({
     event: "join_room",
     room_id: "room101",
     user_id: "viewer999",
     payload: { sdp: offer.sdp, type: offer.type }
   }));
   ```

---

## 📡 সিগন্যালিং প্রোটোকল ও মেসেজ ফরম্যাট (Signaling Protocol Spec)

সব সিগন্যালিং মেসেজ নিম্নলিখিত স্ট্যান্ডার্ড JSON কাঠামোর মাধ্যমে আদান-প্রদান করা হয়:

```json
{
  "event": "EVENT_NAME",
  "room_id": "room-101",
  "user_id": "user-abc",
  "target_user": "optional-target-user-id",
  "payload": {}
}
```

### ইভেন্ট রেফারেন্স টেবিল:

| ইভেন্টের নাম | প্রেরক (Sender) | উদ্দেশ্য ও Payload |
|---|---|---|
| `publish` | Host / Co-Host | লাইভ ব্রডকাস্ট শুরু করার জন্য SDP Offer পাঠানো। Payload: `{ sdp, type }` |
| `join_room` | Viewer | লাইভ দেখার সাবস্ক্রিপশন শুরু করতে SDP Offer পাঠানো। Payload: `{ sdp, type }` |
| `answer` / `sdp_answer` | Server / Client | SDP Offer-এর জবাবে SDP Answer পাঠানো। Payload: `{ sdp, type }` |
| `ice` | Client / Server | ICE Candidate আদান-প্রদান। Payload: `{ candidate, sdpMid, sdpMLineIndex }` |
| `chat` | Any Client | লাইভ রুমে টেক্সট চ্যাট ব্রডকাস্ট। Payload: `{ text: "Hello guys!" }` |
| `gift` | Viewer | হোস্টকে গিফট পাঠানো। Payload: `{ coins: 50 }` |
| `gift_received` | Server -> All | গিফট রিসিভ হওয়ার নোটিফিকেশন। Payload: `{ sender_id, coins, new_score }` |
| `seat_request` | Viewer -> Host | কো-হোস্ট সিটে বসার জন্য হোস্টের কাছে রিকোয়েস্ট। |
| `seat_accept` | Host -> Viewer | ভিউয়ারের সিট রিকোয়েস্ট গ্রহণ। `target_user`: ভিউয়ারের ID। |
| `set_main_seat` | Host -> All | নির্দিষ্ট কো-হোস্টকে মেইন স্ক্রিনে পিন করা। Payload: `{ target_id }` |
| `start_pk` / `stop_pk` | Host / Admin | দুটি রুমের মধ্যে পিকে ব্যাটল শুরু বা বন্ধ করা। Payload: `{ room_1, room_2 }` |
| `viewer_update` | Server -> All | বর্তমান লাইভ ভিউয়ার সংখ্যা ও লিস্ট। Payload: `{ total_viewers, viewers_list }` |
| `room_closed` | Server -> All | হোস্ট রুম বন্ধ করলে সব ভিউয়ারকে নোটিফাই করা। |

---

## 🛠️ নতুন ভাষার জন্য Client SDK বানানোর পূর্ণাঙ্গ গাইড (Build SDK in Any Language)

যেকোনো নতুন প্ল্যাটফর্ম বা প্রোগ্রামিং ল্যাঙ্গুয়েজে (যেমন: Flutter/Dart, React Native, Swift, Kotlin, Python, C++) SDK তৈরি করার স্ট্যান্ডার্ড ৩-লেয়ার আর্কিটেকচার:

```
+-------------------------------------------------------------+
| Layer 1: WebSocket Transport & JSON Parser                  |
|          - Manages connection, heartbeat, authentication    |
+-------------------------------------------------------------+
| Layer 2: WebRTC PeerConnection State Machine               |
|          - Local/Remote SDP Exchange, ICE gathering, Tracks |
+-------------------------------------------------------------+
| Layer 3: Reactive Event Dispatcher (Observable / Stream)    |
|          - Emits: onTrack, onChat, onGift, onPKBattle, etc. |
+-------------------------------------------------------------+
```

---

### 1. Flutter (Dart) SDK Implementation

Flutter-এ SDK তৈরির জন্য `flutter_webrtc` এবং `web_socket_channel` প্যাকেজ ব্যবহার করুন।

```dart
// live_media_core.dart
import 'dart:async';
import 'dart:convert';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

class LiveMediaCore {
  final String wsUrl;
  final String token;
  final String roomId;
  final String userId;
  final String role; // 'host' or 'viewer'

  WebSocketChannel? _channel;
  RTCPeerConnection? _peerConnection;
  MediaStream? localStream;
  MediaStream? remoteStream;

  // Stream Controllers for Events
  final _onTrackController = StreamController<MediaStreamTrack>.broadcast();
  final _onChatController = StreamController<Map<String, dynamic>>.broadcast();
  final _onViewerCountController = StreamController<int>.broadcast();

  Stream<MediaStreamTrack> get onTrack => _onTrackController.stream;
  Stream<Map<String, dynamic>> get onChat => _onChatController.stream;
  Stream<int> get onViewerCount => _onViewerCountController.stream;

  LiveMediaCore({
    required this.wsUrl,
    required this.token,
    required this.roomId,
    required this.userId,
    this.role = 'viewer',
  });

  // ১. সিগন্যালিং সার্ভারে কানেক্ট
  Future<void> connect() async {
    final uri = Uri.parse('$wsUrl?token=${Uri.encodeComponent(token)}');
    _channel = WebSocketChannel.connect(uri);

    _channel!.stream.listen((message) {
      _handleSignalingMessage(jsonDecode(message as String));
    });
  }

  // ২. ভিউয়ার হিসেবে জয়েন
  Future<void> joinAsViewer(List<Map<String, dynamic>> iceServers) async {
    _peerConnection = await createPeerConnection({
      'iceServers': iceServers,
      'sdpSemantics': 'unified-plan',
    });

    remoteStream = await createLocalMediaStream('remote_stream');

    _peerConnection!.onTrack = (RTCTrackEvent event) {
      if (event.track.kind == 'video' || event.track.kind == 'audio') {
        remoteStream!.addTrack(event.track);
        _onTrackController.add(event.track);
      }
    };

    _peerConnection!.onIceCandidate = (RTCIceCandidate candidate) {
      _send('ice', candidate.toMap());
    };

    // Recvonly transceivers
    await _peerConnection!.addTransceiver(
      kind: RTCRtpMediaType.RTCRtpMediaTypeVideo,
      init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly),
    );
    await _peerConnection!.addTransceiver(
      kind: RTCRtpMediaType.RTCRtpMediaTypeAudio,
      init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly),
    );

    RTCSessionDescription offer = await _peerConnection!.createOffer();
    await _peerConnection!.setLocalDescription(offer);

    _send('join_room', {'sdp': offer.sdp, 'type': offer.type});
  }

  // ৩. মেসেজ হ্যান্ডলার
  void _handleSignalingMessage(Map<String, dynamic> msg) async {
    final event = msg['event'];
    final payload = msg['payload'];

    switch (event) {
      case 'answer':
        await _peerConnection?.setRemoteDescription(
          RTCSessionDescription(payload['sdp'], payload['type']),
        );
        break;
      case 'ice':
        await _peerConnection?.addCandidate(
          RTCIceCandidate(payload['candidate'], payload['sdpMid'], payload['sdpMLineIndex']),
        );
        break;
      case 'chat':
        _onChatController.add(msg);
        break;
      case 'viewer_update':
        _onViewerCountController.add(payload['total_viewers'] ?? msg['total_viewers'] ?? 0);
        break;
    }
  }

  void _send(String event, dynamic payload) {
    _channel?.sink.add(jsonEncode({
      'event': event,
      'room_id': roomId,
      'user_id': userId,
      'payload': payload,
    }));
  }

  void sendChat(String text) => _send('chat', {'text': text});
  void sendGift(int coins) => _send('gift', {'coins': coins});

  void dispose() {
    _channel?.sink.close();
    _peerConnection?.close();
    _onTrackController.close();
    _onChatController.close();
    _onViewerCountController.close();
  }
}
```

---

### 2. React Native / TypeScript SDK Implementation

`react-native-webrtc` ব্যবহার করে তৈরি করা টাইপস্ক্রিপ্ট ক্লাস:

```typescript
// LiveMediaSDK.ts
import { RTCPeerConnection, RTCSessionDescription, RTCIceCandidate, MediaStream } from 'react-native-webrtc';
import EventEmitter from 'events';

export interface SDKOptions {
  wsUrl: string;
  token: string;
  roomId: string;
  userId: string;
  iceServers: Array<{ urls: string | string[]; username?: string; credential?: string }>;
}

export class LiveMediaSDK extends EventEmitter {
  private ws: WebSocket | null = null;
  private pc: RTCPeerConnection | null = null;
  private options: SDKOptions;

  constructor(options: SDKOptions) {
    super();
    this.options = options;
  }

  public async connect(): Promise<void> {
    const url = `${this.options.wsUrl}?token=${encodeURIComponent(this.options.token)}`;
    this.ws = new WebSocket(url);

    this.ws.onmessage = async (event) => {
      const msg = JSON.parse(event.data);
      await this.handleMessage(msg);
    };
  }

  public async joinAsViewer(): Promise<void> {
    this.pc = new RTCPeerConnection({ iceServers: this.options.iceServers });

    this.pc.onicecandidate = (e) => {
      if (e.candidate) {
        this.send('ice', e.candidate.toJSON());
      }
    };

    (this.pc as any).ontrack = (event: any) => {
      this.emit('onTrack', event.streams[0] || event.track);
    };

    const offer = await this.pc.createOffer({ offerToReceiveVideo: true, offerToReceiveAudio: true });
    await this.pc.setLocalDescription(offer);

    this.send('join_room', { sdp: offer.sdp, type: offer.type });
  }

  private async handleMessage(msg: any) {
    switch (msg.event) {
      case 'answer':
        await this.pc?.setRemoteDescription(new RTCSessionDescription(msg.payload));
        break;
      case 'ice':
        await this.pc?.addIceCandidate(new RTCIceCandidate(msg.payload));
        break;
      case 'chat':
        this.emit('onChat', msg.payload);
        break;
    }
  }

  private send(event: string, payload: any) {
    this.ws?.send(JSON.stringify({
      event,
      room_id: this.options.roomId,
      user_id: this.options.userId,
      payload
    }));
  }
}
```

---

### 3. iOS (Swift) SDK Implementation

iOS-এ `WebRTC.framework` এবং `URLSessionWebSocketTask` দিয়ে যেভাবে করবেন:

```swift
import Foundation
import WebRTC

public class LiveMediaCore: NSObject {
    private var webSocketTask: URLSessionWebSocketTask?
    private var peerConnection: RTCPeerConnection?
    private let peerConnectionFactory = RTCPeerConnectionFactory()
    
    public func connect(wsUrl: String, token: String) {
        guard let url = URL(string: "\(wsUrl)?token=\(token)") else { return }
        let session = URLSession(configuration: .default)
        webSocketTask = session.webSocketTask(with: url)
        webSocketTask?.resume()
        listenMessages()
    }
    
    private func listenMessages() {
        webSocketTask?.receive { [weak self] result in
            switch result {
            case .success(let message):
                if case .string(let text) = message {
                    self?.handleMessage(text: text)
                }
                self?.listenMessages()
            case .failure(let error):
                print("WebSocket error: \(error)")
            }
        }
    }
    
    private func handleMessage(text: String) {
        guard let data = text.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let event = json["event"] as? String else { return }
        
        if event == "answer", let payload = json["payload"] as? [String: Any],
           let sdp = payload["sdp"] as? String {
            let sessionDescription = RTCSessionDescription(type: .answer, sdp: sdp)
            peerConnection?.setRemoteDescription(sessionDescription, completionHandler: { _ in })
        }
    }
}
```

---

### 4. Android (Kotlin) SDK Implementation

`google-webrtc` (org.webrtc) এবং `OkHttp WebSocket` ব্যবহার করে:

```kotlin
package com.livemedia.sdk

import okhttp3.*
import org.json.JSONObject
import org.webrtc.*

class LiveMediaCore(
    private val wsUrl: String,
    private val token: String,
    private val roomId: String,
    private val userId: String
) {
    private var webSocket: WebSocket? = null
    private var peerConnection: RTCPeerConnection? = null
    private val okHttpClient = OkHttpClient()

    fun connect() {
        val request = Request.Builder().url("$wsUrl?token=$token").build()
        webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) {
                handleSignalingMessage(JSONObject(text))
            }
        })
    }

    private fun handleSignalingMessage(json: JSONObject) {
        when (json.getString("event")) {
            "answer" -> {
                val payload = json.getJSONObject("payload")
                val sdp = SessionDescription(SessionDescription.Type.ANSWER, payload.getString("sdp"))
                peerConnection?.setRemoteDescription(SimpleSdpObserver(), sdp)
            }
            "ice" -> {
                val payload = json.getJSONObject("payload")
                val candidate = IceCandidate(
                    payload.getString("sdpMid"),
                    payload.getInt("sdpMLineIndex"),
                    payload.getString("candidate")
                )
                peerConnection?.addIceCandidate(candidate)
            }
        }
    }

    private class SimpleSdpObserver : SdpObserver {
        override fun onCreateSuccess(p0: SessionDescription?) {}
        override fun onSetSuccess() {}
        override fun onCreateFailure(p0: String?) {}
        override fun onSetFailure(p0: String?) {}
    }
}
```

---

## 🌐 REST API Endpoints

| মেথড | এন্ডপয়েন্ট | বিবরণ |
|---|---|---|
| `GET` | `/health` | সার্ভারের স্বাস্থ্য ও সক্রিয় রুমের সংখ্যা যাচাই করে। |
| `GET` | `/api/ice-servers` | ক্লায়েন্টের জন্য ডাইনামিক STUN/TURN ক্রেডেনশিয়াল প্রদান করে। |
| `GET` | `/auth/demo-token?user_id=...&role=...&room_id=...` | টেস্টিংয়ের জন্য সাইন করা JWT টোকেন তৈরি করে। |
| `GET` | `/rooms` | বর্তমানে সক্রিয় সব রুম ও ভিউয়ার সংখ্যার তালিকা। |
| `GET` | `/room/:id` | নির্দিষ্ট একটি রুমের বিস্তারিত সারাংশ। |
| `WS`  | `/ws?token=JWT_TOKEN` | রিয়েলটাইম সিগন্যালিং ও WebRTC নেগোসিয়েশন সকেট। |

---

## ⚡ সার্ভার রান এবং ডেপ্লয়মেন্ট গাইড (Running & Deployment)

### পদ্ধতি ১: লোকাল মেশিনে রান করা (Go)
```bash
# ডিপেন্ডেন্সি ডাউনলোড করুন
go mod tidy

# সার্ভার স্টার্ট করুন
go run cmd/main.go
```
সার্ভারটি চালু হলে ব্রাউজারে `http://localhost:8080` ওপেন করলে বিল্ট-ইন টেস্ট UI দেখতে পাবেন।

---

### পদ্ধতি ২: Docker Compose দিয়ে রান করা (Redis + Server)
প্রোডাকশনে ক্লাস্টারিং এবং ফুল স্ট্যাক এক কমান্ডে চালানোর জন্য:

```bash
docker-compose up -d --build
```

### পরিবেশের চলকসমূহ (Environment Variables):

| ভেরিয়েবল | ডিফল্ট মান | বিবরণ |
|---|---|---|
| `PORT` | `8080` | HTTP ও WebSocket লিসেনিং পোর্ট |
| `PUBLIC_IP` | `127.0.0.1` | WebRTC ICE হোস্ট ক্যান্ডিডেটের পাবলিক/লোকাল আইপি |
| `REDIS_ADDR` | `localhost:6379` | মাল্টি-সার্ভার ক্লাস্টারিং ও Pub/Sub-এর জন্য Redis ঠিকানা |
| `JWT_SECRET` | `default_secret` | JWT ভ্যালিডেশনের গোপন চাবি |
| `WEBHOOK_URL` | *(ঐচ্ছিক)* | রিয়েল-টাইম ইভেন্ট পোস্ট করার জন্য আপনার মূল ব্যাকএন্ড URL |
| `WEBHOOK_SECRET`| *(ঐচ্ছিক)* | HMAC SHA-256 সিগনেচার ভেরিফিকেশন চাবি |
| `SERVER_ROLE` | `origin` | সার্ভার নোডের ভূমিকা (`origin` অথবা `edge`) |

---

## 🧪 টেস্টিং ও ভ্যালিডেশন (Testing)

সম্পূর্ণ মিডিয়া সার্ভারের ইউনিট টেস্ট ও ইন্টিগ্রেশন টেস্ট চালাতে:

```bash
# সব প্যাকেজের ইউনিট টেস্ট রান করুন
go test -v -race ./...

# টেস্ট কভারেজ রিপোর্ট দেখুন
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

---

## 📄 লাইসেন্স (License)

MIT License © 2026 Live Media Server Team. Open-source and free for commercial and personal usage.
