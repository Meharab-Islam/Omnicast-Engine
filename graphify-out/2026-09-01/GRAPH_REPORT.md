# Graph Report - go_media_server  (2026-09-01)

## Corpus Check
- 97 files · ~98,052 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1056 nodes · 2190 edges · 45 communities (37 shown, 8 thin omitted)
- Extraction: 91% EXTRACTED · 9% INFERRED · 0% AMBIGUOUS · INFERRED: 190 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `d36b66fe`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Room
- SignalingMessage
- TimestampAdjuster
- RedisBroker
- LiveMediaCore
- OmnicastNetworkStack
- LiveRoomClient
- TrackSwitcher
- signaling.go
- webrtc.go
- testing.T
- RoomManager
- sync.RWMutex
- 🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server
- SFUEngine
- github.com/pion/rtp.Packet
- LiveRoomClient
- main
- omnicast
- FanOutDispatcher
- api/auth.go
- entrypoint.sh
- PKManager
- WorkerPool
- ActiveSpeakerDetector
- NewHub
- TestMetricsTracking
- NewSequenceNumberAdjuster
- HandleHostConnection
- NewTrackSwitcher
- time.Duration
- syncRoomStateInternal
- WebhookClient
- room_manager.go
- InitOmnicastMediaEngine
- NewRoom
- InitWebRTC
- Client
- Hub
- GenerateLiveKitToken
- NewBandwidthEstimator
- turn_auth.go
- redis_broker_test.go
- TestStartEmbeddedTURNServer
- GetAppConfig

## God Nodes (most connected - your core abstractions)
1. `Room` - 135 edges
2. `RoomManager` - 58 edges
3. `Client` - 49 edges
4. `RedisBroker` - 47 edges
5. `TrackSwitcher` - 41 edges
6. `SignalingMessage` - 39 edges
7. `NewRoomManager()` - 26 edges
8. `LiveMediaCore` - 26 edges
9. `NewTrackSwitcher()` - 20 edges
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

## Communities (45 total, 8 thin omitted)

### Community 0 - "Room"
Cohesion: 0.05
Nodes (11): time.Timer, Participant, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, NewRoomWithName(), MediaState, RoomState (+3 more)

### Community 1 - "SignalingMessage"
Cohesion: 0.14
Nodes (3): encoding/json.RawMessage, SignalingMessage, WSMessage

### Community 2 - "TimestampAdjuster"
Cohesion: 0.09
Nodes (16): github.com/pion/rtcp.SenderReport, getSenderSSRC(), webrtc.PeerConnection, webrtc.RTPSender, NewTimeSynchronizer(), NtpToTime(), StartPeriodicSenderReports(), TestTimeSynchronizer_LipSyncRewriting() (+8 more)

### Community 3 - "RedisBroker"
Cohesion: 0.06
Nodes (23): MessageHandler, context.CancelFunc, context.Context, github.com/fasthttp/websocket.Conn, FormatViewerSignalingChannel(), RedisBroker, redis.Client, NewRedisBroker() (+15 more)

### Community 5 - "OmnicastNetworkStack"
Cohesion: 0.20
Nodes (11): net.Listener, net.PacketConn, ice.TCPMux, ice.UDPMux, turn.Server, webrtc.SettingEngine, InitOmnicastNetworkLayer(), TestInitOmnicastNetworkLayer() (+3 more)

### Community 6 - "LiveRoomClient"
Cohesion: 0.06
Nodes (5): EventEmitter, LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager

### Community 7 - "TrackSwitcher"
Cohesion: 0.07
Nodes (18): webrtc.TrackLocalStaticRTP, IsKeyframe(), IsVP8Keyframe(), NewSimulcastTrackSwitcher(), NewVP9TrackSwitcher(), TestIsVP8Keyframe(), IsVP9Keyframe(), NewVP9PayloadParser() (+10 more)

### Community 10 - "testing.T"
Cohesion: 0.22
Nodes (18): testing.T, TestRoomManager_ForceEndRoomKillSwitch(), TestPKManager(), NewRoomManager(), TestActiveRoomsWaitGroup_Tracking(), TestAddTrackAndRenegotiate(), TestGetAllRooms(), TestHostCreateRoom_RejectedWhenDraining() (+10 more)

### Community 11 - "RoomManager"
Cohesion: 0.11
Nodes (4): omnicast/internal/api.WebhookDispatcher, webrtc.API, webrtc.TrackLocalStaticRTP, RoomManager

### Community 12 - "sync.RWMutex"
Cohesion: 0.06
Nodes (13): sync.RWMutex, NewABRController(), NewDynacastEngine(), TestABRController_Evaluation(), TestDynacastEngine_SubscriberTracking(), NewViewportManager(), TestViewportManager_DefaultsAndVisibility(), TestViewportManager_ResetAndRemove() (+5 more)

### Community 13 - "🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server"
Cohesion: 0.05
Nodes (36): 🌐 100k+ ডিস্ট্রিবিউটেড ক্লাস্টারিং ও লোড ব্যালেন্সিং (Horizontal Scaling), ⚡ 10k+ ভিউয়ার স্কেলিং ও অপ্টিমাইজেশন (Extreme Performance Optimizations), 1. আল্ট্রা-লো লেটেন্সি WebRTC SFU মিডিয়া ইঞ্জিন, 2. মাল্টি-গেস্ট / কো-হোস্টিং সিট ম্যানেজমেন্ট, 3. লাইভ পিকে ব্যাটল ইঞ্জিন (Live PK Battles), 4. গিফট ইকোনমি ও স্কোর সিস্টেম, 5. ডাইনামিক ইউজার মেটাডাটা (No Backend Code Change Needed), 6. ভিডিও বনাম অডিও-অনলি রুম সিকিউরিটি ফিল্টারিং (+28 more)

### Community 14 - "SFUEngine"
Cohesion: 0.10
Nodes (20): sync.Mutex, GenerateAuthKeyWithSecret(), turn.Server, NewEmbeddedTURNServer(), TestEmbeddedTURNServer_InitializationAndCredentials(), ValidateAndGenerateAuthKey(), ValidateEphemeralCredential(), ice.TCPMux (+12 more)

### Community 15 - "github.com/pion/rtp.Packet"
Cohesion: 0.07
Nodes (19): github.com/pion/rtp.Packet, net.UDPAddr, net.UDPConn, webrtc.TrackLocalStaticRTP, NewLeakyBucketPacer(), TestLeakyBucketPacer(), NewPacketBuffer(), TestPacketBuffer_BasicOperations() (+11 more)

### Community 16 - "LiveRoomClient"
Cohesion: 0.06
Nodes (8): LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager, PKSession, PublishOptions, RoomStateSnapshot, SDKConfig

### Community 17 - "main"
Cohesion: 0.11
Nodes (25): getEnv(), main(), CascadingYAML, CoHostingYAML, Config, InteractionsYAML, ModerationYAML, PKBattleYAML (+17 more)

### Community 19 - "FanOutDispatcher"
Cohesion: 0.15
Nodes (13): github.com/pion/rtp.Header, sync.Pool, webrtc.TrackLocalStaticRTP, NewFanOutDispatcher(), NewSubscriber(), TestFanOutDispatcher_SelectiveFiltering(), TestFanOutDispatcher_SubscribeAndBroadcast(), TestFanOutDispatcher_Unsubscribe() (+5 more)

### Community 20 - "api/auth.go"
Cohesion: 0.15
Nodes (13): ICEServerJSON, TokenRequest, TokenResponse, UserClaims, fiber.Handler, GenerateTokenWithPermissions(), GenerateUserToken(), AuthHandler (+5 more)

### Community 21 - "entrypoint.sh"
Cohesion: 0.40
Nodes (4): REDIS_ADDR, entrypoint.sh script, TURN_REALM, TURN_SECRET

### Community 22 - "PKManager"
Cohesion: 0.21
Nodes (4): PKSession, RoomManager, NewPKManager(), PKManager

### Community 23 - "WorkerPool"
Cohesion: 0.25
Nodes (5): sync.Once, sync.WaitGroup, GetActiveRoomsWaitGroup(), NewWorkerPool(), WorkerPool

### Community 24 - "ActiveSpeakerDetector"
Cohesion: 0.09
Nodes (20): NewActiveSpeakerDetector(), NewActiveSpeakerDetectorWithConfig(), ParseAudioLevel(), TestActiveSpeakerDetector_EMASmoothing(), TestActiveSpeakerDetector_GetTopSpeakers(), TestActiveSpeakerDetector_RemoveSpeaker(), TestActiveSpeakerDetector_StaleSpeaker(), TestActiveSpeakerDetector_UpdateAndGet() (+12 more)

### Community 25 - "NewHub"
Cohesion: 0.15
Nodes (9): ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode(), NewHub(), TestICERestart_SeamlessRenegotiation(), TestLobbyRealTimeUpdates(), TestNativePKBridging_CrossTrackAndTargetedGifting(), TestTrackMuteSignaling() (+1 more)

### Community 26 - "TestMetricsTracking"
Cohesion: 0.20
Nodes (9): AddBytesReceived(), AddBytesSent(), DecActiveParticipants(), DecActiveRooms(), GetSystemSummary(), IncActiveParticipants(), IncActiveRooms(), TestMetricsTracking() (+1 more)

### Community 27 - "NewSequenceNumberAdjuster"
Cohesion: 0.43
Nodes (5): NewSequenceNumberAdjuster(), TestSequenceNumberAdjuster_BasicAndContinuity(), TestSequenceNumberAdjuster_Concurrent(), TestSequenceNumberAdjuster_NextContiguous(), TestSequenceNumberAdjuster_WrapAround()

### Community 28 - "HandleHostConnection"
Cohesion: 0.17
Nodes (16): github.com/pion/rtcp.PictureLossIndication, webrtc.API, webrtc.Configuration, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, HandleCoHostConnection(), HandleHostConnection(), GetPLIPacket() (+8 more)

### Community 29 - "NewTrackSwitcher"
Cohesion: 0.31
Nodes (10): NewTrackSwitcher(), TestIsKeyframe(), TestTrackSwitcher_ContiguousSequenceNumbersOnSVCDrop(), TestTrackSwitcher_DropDeltaFramesUntilKeyframeOnSubscribe(), TestTrackSwitcher_DropHighestSpatialLayerOnCongestion(), TestTrackSwitcher_PendingSwitchAndTargetTrack(), TestTrackSwitcher_SequenceAndTimestampContinuity(), TestTrackSwitcher_SimulcastTracks() (+2 more)

### Community 30 - "time.Duration"
Cohesion: 0.07
Nodes (30): github.com/pion/rtcp.NackPair, github.com/pion/rtcp.ReceiverReport, github.com/pion/rtcp.TransportLayerCC, time.Duration, time.Time, CompactNTP(), ExtractRTTFromReceiverReport(), MonitorBandwidth() (+22 more)

### Community 32 - "WebhookClient"
Cohesion: 0.20
Nodes (10): WebhookClient, WebhookEvent, WebhookEventType, net/http.Client, GenerateSignature(), NewWebhookClient(), NewWebhookDispatcher(), TestWebhookClient_BearerAuthAndWorkerPool() (+2 more)

### Community 33 - "room_manager.go"
Cohesion: 0.25
Nodes (5): IsServerDraining(), SetServerDraining(), TestServerDrainingFlag(), RoomInfo, RoomSummary

### Community 34 - "InitOmnicastMediaEngine"
Cohesion: 0.18
Nodes (11): github.com/pion/interceptor.Registry, webrtc.API, webrtc.PeerConnection, webrtc.RTPSender, webrtc.SettingEngine, InitOmnicastMediaEngine(), NewOmnicastWebRTCAPI(), RouteRTCPFeedback() (+3 more)

### Community 35 - "NewRoom"
Cohesion: 0.21
Nodes (11): NewRoom(), TestRoomCoHostTracks(), TestRoomParticipantsAndPresence(), TestRoomSimulcastTracks(), TestRoomState(), TestRoomTrackSwitcherRegistry(), TestRoomTypeAndStateSync(), TestRoom_BannedUser() (+3 more)

### Community 36 - "InitWebRTC"
Cohesion: 0.31
Nodes (6): TestCascadeManager_Initialization(), webrtc.API, InitWebRTC(), InitWebRTCWithTCPListener(), TestInitWebRTC(), TestInitWebRTCWithTCPListener()

### Community 37 - "Client"
Cohesion: 0.18
Nodes (9): github.com/gofiber/contrib/websocket.Conn, sync/atomic.Int64, RoomManager, webrtc.API, webrtc.PeerConnection, NewClient(), NewClientWithClaims(), Client (+1 more)

### Community 39 - "GenerateLiveKitToken"
Cohesion: 0.22
Nodes (7): LiveKitTokenRequest, LiveKitTokenResponse, GenerateLiveKitToken(), AuthHandler, fiber.Ctx, TestGenerateLiveKitToken_HostPermissions(), TestGenerateLiveKitToken_ViewerPermissions()

### Community 40 - "NewBandwidthEstimator"
Cohesion: 0.31
Nodes (9): EvaluateBitrateLayer(), NewBandwidthEstimator(), TestBandwidthEstimator_AIMD(), TestBandwidthEstimator_ConcurrentAccess(), TestBandwidthEstimator_CongestionAndSpatialLayer(), TestBandwidthEstimator_InitializationAndGetters(), TestBandwidthEstimator_MonitorBandwidth(), TestBandwidthEstimator_ProcessTWCC() (+1 more)

### Community 41 - "turn_auth.go"
Cohesion: 0.33
Nodes (7): GenerateTURNCredentials(), GetDefaultICEServers(), GetDefaultICEServersJSON(), TestGenerateTURNCredentials(), TestGetDefaultICEServers(), webrtc.ICEServer, ICEServerJSON

### Community 42 - "redis_broker_test.go"
Cohesion: 0.50
Nodes (3): TestFormatViewerSignalingChannel(), TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations()

## Knowledge Gaps
- **42 isolated node(s):** `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET`, `TURN_REALM`, `omnicast` (+37 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **8 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `NewRoom`, `RedisBroker`, `RoomManager`, `sync.RWMutex`, `GetAppConfig`, `SFUEngine`, `github.com/pion/rtp.Packet`, `TestMetricsTracking`, `HandleHostConnection`, `time.Duration`, `syncRoomStateInternal`?**
  _High betweenness centrality (0.205) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `room_manager.go`, `RedisBroker`, `testing.T`, `sync.RWMutex`, `GetAppConfig`, `SFUEngine`, `PKManager`, `WorkerPool`, `TestMetricsTracking`, `syncRoomStateInternal`?**
  _High betweenness centrality (0.092) - this node is a cross-community bridge._
- **Why does `RedisBroker` connect `RedisBroker` to `RoomManager`, `sync.RWMutex`, `syncRoomStateInternal`?**
  _High betweenness centrality (0.064) - this node is a cross-community bridge._
- **What connects `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET` to the rest of the system?**
  _42 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.05126050420168067 - nodes in this community are weakly interconnected._
- **Should `SignalingMessage` be split into smaller, more focused modules?**
  _Cohesion score 0.14245014245014245 - nodes in this community are weakly interconnected._
- **Should `TimestampAdjuster` be split into smaller, more focused modules?**
  _Cohesion score 0.0928030303030303 - nodes in this community are weakly interconnected._