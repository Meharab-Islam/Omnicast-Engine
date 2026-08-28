# Graph Report - go_media_server  (2026-08-28)

## Corpus Check
- 66 files · ~72,742 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 790 nodes · 1625 edges · 29 communities (24 shown, 5 thin omitted)
- Extraction: 93% EXTRACTED · 7% INFERRED · 0% AMBIGUOUS · INFERRED: 108 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `d7d43ff4`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Room
- Client
- testing.T
- RedisBroker
- LiveMediaCore
- WebhookDispatcher
- LiveRoomClient
- TrackSwitcher
- signaling.go
- webrtc.go
- turn_auth.go
- RoomManager
- api/auth.go
- 🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server
- time.Duration
- PacketBuffer
- LiveRoomClient
- main
- omnicast
- FanOutDispatcher
- sync.RWMutex
- entrypoint.sh
- TimestampAdjuster
- NewRoomManager
- ActiveSpeakerDetector
- NewActiveSpeakerDetector
- AudioForwarder
- NewBandwidthEstimator
- InitWebRTC

## God Nodes (most connected - your core abstractions)
1. `Room` - 117 edges
2. `RoomManager` - 48 edges
3. `Client` - 43 edges
4. `RedisBroker` - 40 edges
5. `SignalingMessage` - 31 edges
6. `TrackSwitcher` - 28 edges
7. `LiveMediaCore` - 25 edges
8. `LiveRoomClient` - 17 edges
9. `BandwidthEstimator` - 16 edges
10. `main()` - 15 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `NewAuthHandler()`  [EXTRACTED]
  cmd/main.go → internal/api/auth.go
- `main()` --calls--> `NewWebhookDispatcher()`  [EXTRACTED]
  cmd/main.go → internal/api/webhook.go
- `main()` --calls--> `NewRedisBroker()`  [EXTRACTED]
  cmd/main.go → internal/broker/redis_broker.go
- `main()` --calls--> `NewClientWithClaims()`  [EXTRACTED]
  cmd/main.go → internal/signaling/client.go
- `main()` --calls--> `NewHub()`  [EXTRACTED]
  cmd/main.go → internal/signaling/hub.go

## Import Cycles
- None detected.

## Communities (29 total, 5 thin omitted)

### Community 0 - "Room"
Cohesion: 0.06
Nodes (10): time.Timer, Participant, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, NewRoomWithName(), MediaState, RoomState (+2 more)

### Community 1 - "Client"
Cohesion: 0.06
Nodes (28): encoding/json.RawMessage, github.com/gofiber/contrib/websocket.Conn, GetAppConfig(), SignalingMessage, ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode(), RoomManager (+20 more)

### Community 2 - "testing.T"
Cohesion: 0.22
Nodes (14): testing.T, TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations(), NewRoom(), TestRoomCoHostTracks(), TestRoomParticipantsAndPresence(), TestRoomSimulcastTracks(), TestRoomState() (+6 more)

### Community 3 - "RedisBroker"
Cohesion: 0.09
Nodes (14): MessageHandler, context.CancelFunc, context.Context, github.com/fasthttp/websocket.Conn, sync.Mutex, RedisBroker, NewRedisBroker(), webrtc.API (+6 more)

### Community 5 - "WebhookDispatcher"
Cohesion: 0.12
Nodes (12): WebhookEvent, WebhookEventType, net/http.Client, sync.Once, sync.WaitGroup, GenerateSignature(), WebhookDispatcher, NewWebhookDispatcher() (+4 more)

### Community 6 - "LiveRoomClient"
Cohesion: 0.06
Nodes (5): EventEmitter, LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager

### Community 7 - "TrackSwitcher"
Cohesion: 0.07
Nodes (17): NewSequenceNumberAdjuster(), TestSequenceNumberAdjuster_BasicAndContinuity(), TestSequenceNumberAdjuster_Concurrent(), TestSequenceNumberAdjuster_WrapAround(), webrtc.TrackLocalStaticRTP, IsKeyframe(), IsVP8Keyframe(), NewSimulcastTrackSwitcher() (+9 more)

### Community 10 - "turn_auth.go"
Cohesion: 0.33
Nodes (7): GenerateTURNCredentials(), GetDefaultICEServers(), GetDefaultICEServersJSON(), TestGenerateTURNCredentials(), TestGetDefaultICEServers(), webrtc.ICEServer, ICEServerJSON

### Community 11 - "RoomManager"
Cohesion: 0.07
Nodes (11): github.com/pion/webrtc/v3.TrackLocalStaticRTP, PKSession, RoomManager, NewPKManager(), broadcastRawToRoomInternal(), broadcastToRoomInternal(), syncRoomStateInternal(), PKManager (+3 more)

### Community 12 - "api/auth.go"
Cohesion: 0.19
Nodes (10): AuthHandler, ICEServerJSON, TokenRequest, TokenResponse, UserClaims, fiber.Ctx, GenerateUserToken(), jwt.RegisteredClaims (+2 more)

### Community 13 - "🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server"
Cohesion: 0.05
Nodes (36): 🌐 100k+ ডিস্ট্রিবিউটেড ক্লাস্টারিং ও লোড ব্যালেন্সিং (Horizontal Scaling), ⚡ 10k+ ভিউয়ার স্কেলিং ও অপ্টিমাইজেশন (Extreme Performance Optimizations), 1. আল্ট্রা-লো লেটেন্সি WebRTC SFU মিডিয়া ইঞ্জিন, 2. মাল্টি-গেস্ট / কো-হোস্টিং সিট ম্যানেজমেন্ট, 3. লাইভ পিকে ব্যাটল ইঞ্জিন (Live PK Battles), 4. গিফট ইকোনমি ও স্কোর সিস্টেম, 5. ডাইনামিক ইউজার মেটাডাটা (No Backend Code Change Needed), 6. ভিডিও বনাম অডিও-অনলি রুম সিকিউরিটি ফিল্টারিং (+28 more)

### Community 14 - "time.Duration"
Cohesion: 0.09
Nodes (17): github.com/pion/rtcp.ReceiverReport, github.com/pion/rtcp.TransportLayerCC, time.Duration, time.Time, CompactNTP(), ExtractRTTFromReceiverReport(), MonitorBandwidth(), ParseTWCCLoss() (+9 more)

### Community 15 - "PacketBuffer"
Cohesion: 0.18
Nodes (6): github.com/pion/rtp.Packet, NewPacketBuffer(), TestPacketBuffer_BasicOperations(), TestPacketBuffer_CircularOverflow(), TestPacketBuffer_ConcurrentAccess(), PacketBuffer

### Community 16 - "LiveRoomClient"
Cohesion: 0.06
Nodes (8): LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager, PKSession, PublishOptions, RoomStateSnapshot, SDKConfig

### Community 17 - "main"
Cohesion: 0.12
Nodes (25): getEnv(), main(), CascadingYAML, CoHostingYAML, Config, InteractionsYAML, ModerationYAML, PKBattleYAML (+17 more)

### Community 19 - "FanOutDispatcher"
Cohesion: 0.15
Nodes (13): github.com/pion/rtp.Header, sync.Pool, webrtc.TrackLocalStaticRTP, NewFanOutDispatcher(), NewSubscriber(), TestFanOutDispatcher_SelectiveFiltering(), TestFanOutDispatcher_SubscribeAndBroadcast(), TestFanOutDispatcher_Unsubscribe() (+5 more)

### Community 20 - "sync.RWMutex"
Cohesion: 0.07
Nodes (24): github.com/pion/rtcp.NackPair, sync.RWMutex, NewABRController(), NewDynacastEngine(), TestABRController_Evaluation(), TestDynacastEngine_SubscriberTracking(), BuildNackPairs(), ExtractLostSequenceNumbers() (+16 more)

### Community 21 - "entrypoint.sh"
Cohesion: 0.40
Nodes (4): REDIS_ADDR, entrypoint.sh script, TURN_REALM, TURN_SECRET

### Community 22 - "TimestampAdjuster"
Cohesion: 0.15
Nodes (5): NewTimestampAdjuster(), TestTimestampAdjuster_BasicAndContinuity(), TestTimestampAdjuster_Concurrent(), TestTimestampAdjuster_WrapAround(), TimestampAdjuster

### Community 23 - "NewRoomManager"
Cohesion: 0.23
Nodes (10): TestRoomManager_ForceEndRoomKillSwitch(), TestPKManager(), NewRoomManager(), TestAddTrackAndRenegotiate(), TestGetAllRooms(), TestPKBattleFullLifecycle(), TestRoomManager(), TestRoomStateSyncAndGiftScore() (+2 more)

### Community 24 - "ActiveSpeakerDetector"
Cohesion: 0.19
Nodes (7): NewActiveSpeakerDetectorWithConfig(), ParseAudioLevel(), TestActiveSpeakerDetector_StaleSpeaker(), TestParseAudioLevel(), ActiveSpeakerDetector, SpeakerInfo, speakerState

### Community 25 - "NewActiveSpeakerDetector"
Cohesion: 0.29
Nodes (11): NewActiveSpeakerDetector(), TestActiveSpeakerDetector_EMASmoothing(), TestActiveSpeakerDetector_GetTopSpeakers(), TestActiveSpeakerDetector_RemoveSpeaker(), TestActiveSpeakerDetector_UpdateAndGet(), NewAudioForwarder(), TestAudioForwarder_FallbackWhenNoOneSpeaking(), TestAudioForwarder_GetActiveSpeakers() (+3 more)

### Community 27 - "NewBandwidthEstimator"
Cohesion: 0.31
Nodes (9): EvaluateBitrateLayer(), NewBandwidthEstimator(), TestBandwidthEstimator_AIMD(), TestBandwidthEstimator_ConcurrentAccess(), TestBandwidthEstimator_InitializationAndGetters(), TestBandwidthEstimator_MonitorBandwidth(), TestBandwidthEstimator_ProcessReceiverReport(), TestBandwidthEstimator_ProcessTWCC() (+1 more)

### Community 28 - "InitWebRTC"
Cohesion: 0.40
Nodes (3): webrtc.API, InitWebRTC(), TestInitWebRTC()

## Knowledge Gaps
- **39 isolated node(s):** `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET`, `TURN_REALM`, `omnicast` (+34 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `Client`, `testing.T`, `RedisBroker`, `RoomManager`, `time.Duration`, `sync.RWMutex`?**
  _High betweenness centrality (0.206) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `Client`, `RedisBroker`, `WebhookDispatcher`, `sync.RWMutex`, `NewRoomManager`?**
  _High betweenness centrality (0.108) - this node is a cross-community bridge._
- **Why does `RedisBroker` connect `RedisBroker` to `RoomManager`, `sync.RWMutex`?**
  _High betweenness centrality (0.057) - this node is a cross-community bridge._
- **What connects `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET` to the rest of the system?**
  _39 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.06079207920792079 - nodes in this community are weakly interconnected._
- **Should `Client` be split into smaller, more focused modules?**
  _Cohesion score 0.06277665995975855 - nodes in this community are weakly interconnected._
- **Should `RedisBroker` be split into smaller, more focused modules?**
  _Cohesion score 0.08865248226950355 - nodes in this community are weakly interconnected._