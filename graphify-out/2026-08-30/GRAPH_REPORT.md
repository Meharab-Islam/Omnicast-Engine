# Graph Report - go_media_server  (2026-08-30)

## Corpus Check
- 81 files · ~91,326 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 963 nodes · 2036 edges · 38 communities (32 shown, 6 thin omitted)
- Extraction: 92% EXTRACTED · 8% INFERRED · 0% AMBIGUOUS · INFERRED: 166 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `982c2e72`
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
- NewRoomManager
- RoomManager
- NewRoom
- 🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server
- EmbeddedTURNServer
- PacketBuffer
- LiveRoomClient
- config.go
- omnicast
- FanOutDispatcher
- sync.RWMutex
- entrypoint.sh
- PKManager
- WorkerPool
- ActiveSpeakerDetector
- syncRoomStateInternal
- TestMetricsTracking
- time.Duration
- main
- testing.T
- api/auth.go
- signaling/auth.go
- InitWebRTC
- room_manager.go
- NewSequenceNumberAdjuster
- turn_auth.go
- NewVP9PayloadParser
- ParseSignalingMessage

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
- `main()` --calls--> `LoadOrGenerateConfig()`  [EXTRACTED]
  cmd/main.go → internal/config/config.go
- `main()` --calls--> `PrintStartupBanner()`  [EXTRACTED]
  cmd/main.go → internal/config/config.go

## Import Cycles
- None detected.

## Communities (38 total, 6 thin omitted)

### Community 0 - "Room"
Cohesion: 0.05
Nodes (10): time.Timer, Participant, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, MediaState, RoomState, CoHostMedia (+2 more)

### Community 1 - "Client"
Cohesion: 0.06
Nodes (26): encoding/json.RawMessage, github.com/gofiber/contrib/websocket.Conn, sync/atomic.Int64, GetAppConfig(), SignalingMessage, RoomManager, webrtc.API, webrtc.PeerConnection (+18 more)

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
Cohesion: 0.08
Nodes (11): webrtc.TrackLocalStaticRTP, IsKeyframe(), IsVP8Keyframe(), NewSimulcastTrackSwitcher(), NewVP9TrackSwitcher(), IsVP9Keyframe(), ParseVP9Descriptor(), webrtc.TrackRemote (+3 more)

### Community 10 - "NewRoomManager"
Cohesion: 0.18
Nodes (16): TestRoomManager_ForceEndRoomKillSwitch(), NewRoomManager(), SetServerDraining(), TestActiveRoomsWaitGroup_Tracking(), TestAddTrackAndRenegotiate(), TestGetAllRooms(), TestHostCreateRoom_RejectedWhenDraining(), TestNodeB_RelaysAnswerToViewerWebSocket() (+8 more)

### Community 11 - "RoomManager"
Cohesion: 0.11
Nodes (4): omnicast/internal/api.WebhookDispatcher, webrtc.API, webrtc.TrackLocalStaticRTP, RoomManager

### Community 12 - "NewRoom"
Cohesion: 0.18
Nodes (12): NewRoom(), NewRoomWithName(), TestRoomCoHostTracks(), TestRoomParticipantsAndPresence(), TestRoomSimulcastTracks(), TestRoomState(), TestRoomTrackSwitcherRegistry(), TestRoomTypeAndStateSync() (+4 more)

### Community 13 - "🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server"
Cohesion: 0.05
Nodes (36): 🌐 100k+ ডিস্ট্রিবিউটেড ক্লাস্টারিং ও লোড ব্যালেন্সিং (Horizontal Scaling), ⚡ 10k+ ভিউয়ার স্কেলিং ও অপ্টিমাইজেশন (Extreme Performance Optimizations), 1. আল্ট্রা-লো লেটেন্সি WebRTC SFU মিডিয়া ইঞ্জিন, 2. মাল্টি-গেস্ট / কো-হোস্টিং সিট ম্যানেজমেন্ট, 3. লাইভ পিকে ব্যাটল ইঞ্জিন (Live PK Battles), 4. গিফট ইকোনমি ও স্কোর সিস্টেম, 5. ডাইনামিক ইউজার মেটাডাটা (No Backend Code Change Needed), 6. ভিডিও বনাম অডিও-অনলি রুম সিকিউরিটি ফিল্টারিং (+28 more)

### Community 14 - "EmbeddedTURNServer"
Cohesion: 0.27
Nodes (8): GenerateAuthKeyWithSecret(), NewEmbeddedTURNServer(), TestEmbeddedTURNServer_InitializationAndCredentials(), ValidateAndGenerateAuthKey(), ValidateEphemeralCredential(), turn.Server, EmbeddedTURNConfig, EmbeddedTURNServer

### Community 15 - "PacketBuffer"
Cohesion: 0.09
Nodes (14): github.com/pion/rtp.Packet, net.UDPAddr, net.UDPConn, NewPacketBuffer(), TestPacketBuffer_BasicOperations(), TestPacketBuffer_CircularOverflow(), TestPacketBuffer_ConcurrentAccess(), NewUDPRTPForwarder() (+6 more)

### Community 16 - "LiveRoomClient"
Cohesion: 0.06
Nodes (8): LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager, PKSession, PublishOptions, RoomStateSnapshot, SDKConfig

### Community 17 - "config.go"
Cohesion: 0.20
Nodes (16): CascadingYAML, CoHostingYAML, Config, InteractionsYAML, ModerationYAML, PKBattleYAML, RoomManagementYAML, ServerYAML (+8 more)

### Community 19 - "FanOutDispatcher"
Cohesion: 0.15
Nodes (13): github.com/pion/rtp.Header, sync.Pool, webrtc.TrackLocalStaticRTP, NewFanOutDispatcher(), NewSubscriber(), TestFanOutDispatcher_SelectiveFiltering(), TestFanOutDispatcher_SubscribeAndBroadcast(), TestFanOutDispatcher_Unsubscribe() (+5 more)

### Community 20 - "sync.RWMutex"
Cohesion: 0.06
Nodes (13): sync.RWMutex, NewABRController(), NewDynacastEngine(), TestABRController_Evaluation(), TestDynacastEngine_SubscriberTracking(), NewViewportManager(), TestViewportManager_DefaultsAndVisibility(), TestViewportManager_ResetAndRemove() (+5 more)

### Community 21 - "entrypoint.sh"
Cohesion: 0.40
Nodes (4): REDIS_ADDR, entrypoint.sh script, TURN_REALM, TURN_SECRET

### Community 23 - "WorkerPool"
Cohesion: 0.25
Nodes (5): sync.Once, sync.WaitGroup, GetActiveRoomsWaitGroup(), NewWorkerPool(), WorkerPool

### Community 24 - "ActiveSpeakerDetector"
Cohesion: 0.09
Nodes (20): NewActiveSpeakerDetector(), NewActiveSpeakerDetectorWithConfig(), ParseAudioLevel(), TestActiveSpeakerDetector_EMASmoothing(), TestActiveSpeakerDetector_GetTopSpeakers(), TestActiveSpeakerDetector_RemoveSpeaker(), TestActiveSpeakerDetector_StaleSpeaker(), TestActiveSpeakerDetector_UpdateAndGet() (+12 more)

### Community 26 - "TestMetricsTracking"
Cohesion: 0.20
Nodes (9): AddBytesReceived(), AddBytesSent(), DecActiveParticipants(), DecActiveRooms(), GetSystemSummary(), IncActiveParticipants(), IncActiveRooms(), TestMetricsTracking() (+1 more)

### Community 27 - "time.Duration"
Cohesion: 0.06
Nodes (40): github.com/pion/rtcp.NackPair, github.com/pion/rtcp.ReceiverReport, github.com/pion/rtcp.TransportLayerCC, sync.Mutex, time.Duration, time.Time, CompactNTP(), EvaluateBitrateLayer() (+32 more)

### Community 28 - "main"
Cohesion: 0.16
Nodes (9): getEnv(), main(), NewHub(), NewPKManager(), TestPKManager(), TestNativePKBridging_CrossTrackAndTargetedGifting(), TestPKBattleFullLifecycle(), TestTrackMuteSignaling() (+1 more)

### Community 29 - "testing.T"
Cohesion: 0.26
Nodes (14): testing.T, TestFormatViewerSignalingChannel(), TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations(), NewTrackSwitcher(), TestIsKeyframe(), TestIsVP8Keyframe(), TestTrackSwitcher_ContiguousSequenceNumbersOnSVCDrop() (+6 more)

### Community 30 - "api/auth.go"
Cohesion: 0.15
Nodes (13): AuthHandler, ICEServerJSON, TokenRequest, TokenResponse, UserClaims, fiber.Ctx, fiber.Handler, GenerateTokenWithPermissions() (+5 more)

### Community 31 - "signaling/auth.go"
Cohesion: 0.29
Nodes (7): GenerateToken(), GenerateUserToken(), GetJWTSecret(), jwt.RegisteredClaims, TestJWTAuthentication(), ValidateToken(), UserClaims

### Community 32 - "InitWebRTC"
Cohesion: 0.24
Nodes (8): net.Listener, TestNodeA_ProcessRemoteViewerOfferAndAnswer(), TestCascadeManager_Initialization(), webrtc.API, InitWebRTC(), InitWebRTCWithTCPListener(), TestInitWebRTC(), TestInitWebRTCWithTCPListener()

### Community 33 - "room_manager.go"
Cohesion: 0.29
Nodes (3): IsServerDraining(), RoomInfo, RoomSummary

### Community 34 - "NewSequenceNumberAdjuster"
Cohesion: 0.43
Nodes (5): NewSequenceNumberAdjuster(), TestSequenceNumberAdjuster_BasicAndContinuity(), TestSequenceNumberAdjuster_Concurrent(), TestSequenceNumberAdjuster_NextContiguous(), TestSequenceNumberAdjuster_WrapAround()

### Community 35 - "turn_auth.go"
Cohesion: 0.33
Nodes (7): GenerateTURNCredentials(), GetDefaultICEServers(), GetDefaultICEServersJSON(), TestGenerateTURNCredentials(), TestGetDefaultICEServers(), webrtc.ICEServer, ICEServerJSON

### Community 36 - "NewVP9PayloadParser"
Cohesion: 0.48
Nodes (6): NewVP9PayloadParser(), TestVP9PayloadParser_InterFrame(), TestVP9PayloadParser_Keyframe(), TestVP9PayloadParser_LayerIndicesSVC(), TestVP9PayloadParser_PictureID(), TestVP9PayloadParser_Truncated()

### Community 37 - "ParseSignalingMessage"
Cohesion: 0.40
Nodes (4): ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode(), TestICERestart_SeamlessRenegotiation()

## Knowledge Gaps
- **39 isolated node(s):** `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET`, `TURN_REALM`, `omnicast` (+34 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `Client`, `RedisBroker`, `RoomManager`, `NewRoom`, `PacketBuffer`, `sync.RWMutex`, `syncRoomStateInternal`, `TestMetricsTracking`, `time.Duration`?**
  _High betweenness centrality (0.207) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `room_manager.go`, `Client`, `RedisBroker`, `NewRoomManager`, `sync.RWMutex`, `PKManager`, `WorkerPool`, `syncRoomStateInternal`, `TestMetricsTracking`, `time.Duration`?**
  _High betweenness centrality (0.115) - this node is a cross-community bridge._
- **Why does `TrackSwitcher` connect `TrackSwitcher` to `TimestampAdjuster`, `PacketBuffer`, `sync.RWMutex`, `WorkerPool`, `time.Duration`, `testing.T`?**
  _High betweenness centrality (0.065) - this node is a cross-community bridge._
- **What connects `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET` to the rest of the system?**
  _39 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.05244096769520498 - nodes in this community are weakly interconnected._
- **Should `Client` be split into smaller, more focused modules?**
  _Cohesion score 0.057124310288867254 - nodes in this community are weakly interconnected._
- **Should `TimestampAdjuster` be split into smaller, more focused modules?**
  _Cohesion score 0.0928030303030303 - nodes in this community are weakly interconnected._