# Graph Report - go_media_server  (2026-08-30)

## Corpus Check
- 84 files · ~92,282 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 981 nodes · 2070 edges · 39 communities (32 shown, 7 thin omitted)
- Extraction: 92% EXTRACTED · 8% INFERRED · 0% AMBIGUOUS · INFERRED: 173 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `e53747a1`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Room
- Client
- TimestampAdjuster
- RedisBroker
- LiveMediaCore
- WebhookClient
- LiveRoomClient
- TrackSwitcher
- signaling.go
- webrtc.go
- testing.T
- RoomManager
- NewRoom
- 🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server
- sync.Mutex
- github.com/pion/rtp.Packet
- LiveRoomClient
- main
- omnicast
- FanOutDispatcher
- HandleViewerConnectionForRoom
- entrypoint.sh
- PKManager
- WorkerPool
- ActiveSpeakerDetector
- syncRoomStateInternal
- TestMetricsTracking
- GetDynamicRTCConfiguration
- ParseSignalingMessage
- NewTrackSwitcher
- time.Duration
- broadcastToRoomInternal
- InitWebRTC
- room_manager.go
- NewSequenceNumberAdjuster
- redis_broker_test.go
- sync.RWMutex
- .CloseRoomAndNotifyWithReason
- NewBandwidthEstimator

## God Nodes (most connected - your core abstractions)
1. `Room` - 135 edges
2. `RoomManager` - 58 edges
3. `Client` - 49 edges
4. `RedisBroker` - 47 edges
5. `TrackSwitcher` - 41 edges
6. `SignalingMessage` - 38 edges
7. `LiveMediaCore` - 26 edges
8. `NewRoomManager()` - 25 edges
9. `NewTrackSwitcher()` - 19 edges
10. `main()` - 18 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `NewAuthHandler()`  [EXTRACTED]
  cmd/main.go → internal/api/auth.go
- `main()` --calls--> `NewWebhookDispatcher()`  [EXTRACTED]
  cmd/main.go → internal/api/webhook.go
- `main()` --calls--> `NewRedisBroker()`  [EXTRACTED]
  cmd/main.go → internal/broker/redis_broker.go
- `main()` --calls--> `GetSystemSummary()`  [EXTRACTED]
  cmd/main.go → internal/metrics/metrics.go
- `main()` --calls--> `NewClientWithClaims()`  [EXTRACTED]
  cmd/main.go → internal/signaling/client.go

## Import Cycles
- None detected.

## Communities (39 total, 7 thin omitted)

### Community 0 - "Room"
Cohesion: 0.05
Nodes (11): time.Timer, Participant, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, NewRoomWithName(), MediaState, RoomState (+3 more)

### Community 1 - "Client"
Cohesion: 0.07
Nodes (15): encoding/json.RawMessage, github.com/gofiber/contrib/websocket.Conn, sync/atomic.Int64, GetAppConfig(), SignalingMessage, RoomManager, webrtc.API, webrtc.PeerConnection (+7 more)

### Community 2 - "TimestampAdjuster"
Cohesion: 0.09
Nodes (16): github.com/pion/rtcp.SenderReport, getSenderSSRC(), webrtc.PeerConnection, webrtc.RTPSender, NewTimeSynchronizer(), NtpToTime(), StartPeriodicSenderReports(), TestTimeSynchronizer_LipSyncRewriting() (+8 more)

### Community 3 - "RedisBroker"
Cohesion: 0.08
Nodes (15): MessageHandler, context.CancelFunc, context.Context, github.com/fasthttp/websocket.Conn, FormatViewerSignalingChannel(), RedisBroker, NewRedisBroker(), PKSession (+7 more)

### Community 5 - "WebhookClient"
Cohesion: 0.20
Nodes (10): WebhookClient, WebhookEvent, WebhookEventType, net/http.Client, GenerateSignature(), NewWebhookClient(), NewWebhookDispatcher(), TestWebhookClient_BearerAuthAndWorkerPool() (+2 more)

### Community 6 - "LiveRoomClient"
Cohesion: 0.06
Nodes (5): EventEmitter, LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager

### Community 7 - "TrackSwitcher"
Cohesion: 0.07
Nodes (16): webrtc.TrackLocalStaticRTP, IsKeyframe(), NewSimulcastTrackSwitcher(), NewVP9TrackSwitcher(), IsVP9Keyframe(), NewVP9PayloadParser(), ParseVP9Descriptor(), TestVP9PayloadParser_InterFrame() (+8 more)

### Community 10 - "testing.T"
Cohesion: 0.22
Nodes (18): testing.T, TestRoomManager_ForceEndRoomKillSwitch(), TestPKManager(), NewRoomManager(), TestActiveRoomsWaitGroup_Tracking(), TestAddTrackAndRenegotiate(), TestGetAllRooms(), TestHostCreateRoom_RejectedWhenDraining() (+10 more)

### Community 11 - "RoomManager"
Cohesion: 0.11
Nodes (4): omnicast/internal/api.WebhookDispatcher, webrtc.API, webrtc.TrackLocalStaticRTP, RoomManager

### Community 12 - "NewRoom"
Cohesion: 0.21
Nodes (11): NewRoom(), TestRoomCoHostTracks(), TestRoomParticipantsAndPresence(), TestRoomSimulcastTracks(), TestRoomState(), TestRoomTrackSwitcherRegistry(), TestRoomTypeAndStateSync(), TestRoom_BannedUser() (+3 more)

### Community 13 - "🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server"
Cohesion: 0.05
Nodes (36): 🌐 100k+ ডিস্ট্রিবিউটেড ক্লাস্টারিং ও লোড ব্যালেন্সিং (Horizontal Scaling), ⚡ 10k+ ভিউয়ার স্কেলিং ও অপ্টিমাইজেশন (Extreme Performance Optimizations), 1. আল্ট্রা-লো লেটেন্সি WebRTC SFU মিডিয়া ইঞ্জিন, 2. মাল্টি-গেস্ট / কো-হোস্টিং সিট ম্যানেজমেন্ট, 3. লাইভ পিকে ব্যাটল ইঞ্জিন (Live PK Battles), 4. গিফট ইকোনমি ও স্কোর সিস্টেম, 5. ডাইনামিক ইউজার মেটাডাটা (No Backend Code Change Needed), 6. ভিডিও বনাম অডিও-অনলি রুম সিকিউরিটি ফিল্টারিং (+28 more)

### Community 14 - "sync.Mutex"
Cohesion: 0.24
Nodes (9): sync.Mutex, GenerateAuthKeyWithSecret(), NewEmbeddedTURNServer(), TestEmbeddedTURNServer_InitializationAndCredentials(), ValidateAndGenerateAuthKey(), ValidateEphemeralCredential(), turn.Server, EmbeddedTURNConfig (+1 more)

### Community 15 - "github.com/pion/rtp.Packet"
Cohesion: 0.07
Nodes (28): github.com/pion/rtcp.PictureLossIndication, github.com/pion/rtp.Packet, net.UDPAddr, net.UDPConn, webrtc.API, webrtc.Configuration, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP (+20 more)

### Community 16 - "LiveRoomClient"
Cohesion: 0.06
Nodes (8): LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager, PKSession, PublishOptions, RoomStateSnapshot, SDKConfig

### Community 17 - "main"
Cohesion: 0.11
Nodes (25): getEnv(), main(), CascadingYAML, CoHostingYAML, Config, InteractionsYAML, ModerationYAML, PKBattleYAML (+17 more)

### Community 19 - "FanOutDispatcher"
Cohesion: 0.15
Nodes (13): github.com/pion/rtp.Header, sync.Pool, webrtc.TrackLocalStaticRTP, NewFanOutDispatcher(), NewSubscriber(), TestFanOutDispatcher_SelectiveFiltering(), TestFanOutDispatcher_SubscribeAndBroadcast(), TestFanOutDispatcher_Unsubscribe() (+5 more)

### Community 20 - "HandleViewerConnectionForRoom"
Cohesion: 0.11
Nodes (18): github.com/pion/rtcp.NackPair, NewABRController(), NewDynacastEngine(), TestABRController_Evaluation(), TestDynacastEngine_SubscriberTracking(), BuildNackPairs(), ExtractLostSequenceNumbers(), webrtc.API (+10 more)

### Community 21 - "entrypoint.sh"
Cohesion: 0.40
Nodes (4): REDIS_ADDR, entrypoint.sh script, TURN_REALM, TURN_SECRET

### Community 22 - "PKManager"
Cohesion: 0.24
Nodes (3): RoomManager, NewPKManager(), PKManager

### Community 23 - "WorkerPool"
Cohesion: 0.25
Nodes (5): sync.Once, sync.WaitGroup, GetActiveRoomsWaitGroup(), NewWorkerPool(), WorkerPool

### Community 24 - "ActiveSpeakerDetector"
Cohesion: 0.09
Nodes (20): NewActiveSpeakerDetector(), NewActiveSpeakerDetectorWithConfig(), ParseAudioLevel(), TestActiveSpeakerDetector_EMASmoothing(), TestActiveSpeakerDetector_GetTopSpeakers(), TestActiveSpeakerDetector_RemoveSpeaker(), TestActiveSpeakerDetector_StaleSpeaker(), TestActiveSpeakerDetector_UpdateAndGet() (+12 more)

### Community 26 - "TestMetricsTracking"
Cohesion: 0.31
Nodes (8): AddBytesReceived(), AddBytesSent(), DecActiveRooms(), GetSystemSummary(), IncActiveParticipants(), IncActiveRooms(), TestMetricsTracking(), SysSummary

### Community 27 - "GetDynamicRTCConfiguration"
Cohesion: 0.29
Nodes (9): GenerateTURNCredentials(), GetDefaultICEServers(), GetDefaultICEServersJSON(), GetDynamicRTCConfiguration(), webrtc.Configuration, TestGenerateTURNCredentials(), TestGetDefaultICEServers(), webrtc.ICEServer (+1 more)

### Community 28 - "ParseSignalingMessage"
Cohesion: 0.18
Nodes (8): ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode(), NewHub(), TestICERestart_SeamlessRenegotiation(), TestNativePKBridging_CrossTrackAndTargetedGifting(), TestTrackMuteSignaling(), TestZombiePeerGC()

### Community 29 - "NewTrackSwitcher"
Cohesion: 0.26
Nodes (11): IsVP8Keyframe(), NewTrackSwitcher(), TestIsKeyframe(), TestIsVP8Keyframe(), TestTrackSwitcher_ContiguousSequenceNumbersOnSVCDrop(), TestTrackSwitcher_DropHighestSpatialLayerOnCongestion(), TestTrackSwitcher_PendingSwitchAndTargetTrack(), TestTrackSwitcher_SequenceAndTimestampContinuity() (+3 more)

### Community 30 - "time.Duration"
Cohesion: 0.06
Nodes (31): AuthHandler, ICEServerJSON, TokenRequest, TokenResponse, UserClaims, fiber.Ctx, fiber.Handler, github.com/pion/rtcp.ReceiverReport (+23 more)

### Community 32 - "InitWebRTC"
Cohesion: 0.27
Nodes (7): net.Listener, TestCascadeManager_Initialization(), webrtc.API, InitWebRTC(), InitWebRTCWithTCPListener(), TestInitWebRTC(), TestInitWebRTCWithTCPListener()

### Community 33 - "room_manager.go"
Cohesion: 0.25
Nodes (5): IsServerDraining(), SetServerDraining(), TestServerDrainingFlag(), RoomInfo, RoomSummary

### Community 34 - "NewSequenceNumberAdjuster"
Cohesion: 0.43
Nodes (5): NewSequenceNumberAdjuster(), TestSequenceNumberAdjuster_BasicAndContinuity(), TestSequenceNumberAdjuster_Concurrent(), TestSequenceNumberAdjuster_NextContiguous(), TestSequenceNumberAdjuster_WrapAround()

### Community 35 - "redis_broker_test.go"
Cohesion: 0.50
Nodes (3): TestFormatViewerSignalingChannel(), TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations()

### Community 36 - "sync.RWMutex"
Cohesion: 0.07
Nodes (12): sync.RWMutex, webrtc.TrackLocalStaticRTP, NewLeakyBucketPacer(), TestLeakyBucketPacer(), NewViewportManager(), TestViewportManager_DefaultsAndVisibility(), TestViewportManager_ResetAndRemove(), TestViewportManager_SetVisibleTracks() (+4 more)

### Community 39 - "NewBandwidthEstimator"
Cohesion: 0.31
Nodes (9): EvaluateBitrateLayer(), NewBandwidthEstimator(), TestBandwidthEstimator_AIMD(), TestBandwidthEstimator_ConcurrentAccess(), TestBandwidthEstimator_CongestionAndSpatialLayer(), TestBandwidthEstimator_InitializationAndGetters(), TestBandwidthEstimator_MonitorBandwidth(), TestBandwidthEstimator_ProcessTWCC() (+1 more)

## Knowledge Gaps
- **39 isolated node(s):** `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET`, `TURN_REALM`, `omnicast` (+34 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **7 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `RedisBroker`, `sync.RWMutex`, `RoomManager`, `NewRoom`, `sync.Mutex`, `github.com/pion/rtp.Packet`, `HandleViewerConnectionForRoom`, `syncRoomStateInternal`, `TestMetricsTracking`, `time.Duration`, `broadcastToRoomInternal`?**
  _High betweenness centrality (0.205) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `room_manager.go`, `RedisBroker`, `sync.RWMutex`, `.CloseRoomAndNotifyWithReason`, `testing.T`, `sync.Mutex`, `PKManager`, `WorkerPool`, `syncRoomStateInternal`, `TestMetricsTracking`, `broadcastToRoomInternal`?**
  _High betweenness centrality (0.113) - this node is a cross-community bridge._
- **Why does `TrackSwitcher` connect `TrackSwitcher` to `TimestampAdjuster`, `sync.RWMutex`, `github.com/pion/rtp.Packet`, `HandleViewerConnectionForRoom`, `WorkerPool`, `NewTrackSwitcher`?**
  _High betweenness centrality (0.065) - this node is a cross-community bridge._
- **What connects `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET` to the rest of the system?**
  _39 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.05126050420168067 - nodes in this community are weakly interconnected._
- **Should `Client` be split into smaller, more focused modules?**
  _Cohesion score 0.07486338797814207 - nodes in this community are weakly interconnected._
- **Should `TimestampAdjuster` be split into smaller, more focused modules?**
  _Cohesion score 0.0928030303030303 - nodes in this community are weakly interconnected._