# Graph Report - go_media_server  (2026-08-29)

## Corpus Check
- 80 files · ~89,190 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 950 nodes · 2005 edges · 28 communities (23 shown, 5 thin omitted)
- Extraction: 92% EXTRACTED · 8% INFERRED · 0% AMBIGUOUS · INFERRED: 162 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `28820f04`
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
- 🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server
- sync.Mutex
- PacketBuffer
- LiveRoomClient
- main
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
- turn_auth.go

## God Nodes (most connected - your core abstractions)
1. `Room` - 133 edges
2. `RoomManager` - 56 edges
3. `Client` - 48 edges
4. `RedisBroker` - 47 edges
5. `TrackSwitcher` - 41 edges
6. `SignalingMessage` - 36 edges
7. `LiveMediaCore` - 26 edges
8. `NewRoomManager()` - 24 edges
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

## Communities (28 total, 5 thin omitted)

### Community 0 - "Room"
Cohesion: 0.05
Nodes (10): time.Timer, Participant, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, NewRoomWithName(), MediaState, CoHostMedia (+2 more)

### Community 1 - "Client"
Cohesion: 0.06
Nodes (29): encoding/json.RawMessage, github.com/gofiber/contrib/websocket.Conn, sync/atomic.Int64, GetAppConfig(), SignalingMessage, ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode() (+21 more)

### Community 2 - "TimestampAdjuster"
Cohesion: 0.06
Nodes (24): github.com/pion/rtcp.SenderReport, time.Time, CanSendPLI(), ForceSendPLI(), NewPLIThrottler(), TestPLIThrottler_BasicThrottling(), TestPLIThrottler_ConcurrentAccess(), TestPLIThrottler_ResetAndClear() (+16 more)

### Community 3 - "RedisBroker"
Cohesion: 0.08
Nodes (14): MessageHandler, context.Context, github.com/fasthttp/websocket.Conn, FormatViewerSignalingChannel(), RedisBroker, NewRedisBroker(), RoomState, webrtc.API (+6 more)

### Community 5 - "WebhookClient"
Cohesion: 0.20
Nodes (10): WebhookClient, WebhookEvent, WebhookEventType, net/http.Client, GenerateSignature(), NewWebhookClient(), NewWebhookDispatcher(), TestWebhookClient_BearerAuthAndWorkerPool() (+2 more)

### Community 6 - "LiveRoomClient"
Cohesion: 0.06
Nodes (5): EventEmitter, LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager

### Community 7 - "TrackSwitcher"
Cohesion: 0.07
Nodes (17): webrtc.TrackLocalStaticRTP, IsKeyframe(), IsVP8Keyframe(), NewSimulcastTrackSwitcher(), NewVP9TrackSwitcher(), IsVP9Keyframe(), NewVP9PayloadParser(), ParseVP9Descriptor() (+9 more)

### Community 10 - "testing.T"
Cohesion: 0.05
Nodes (61): net.Listener, testing.T, TestFormatViewerSignalingChannel(), TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations(), NewRoom(), TestRoomCoHostTracks(), TestRoomParticipantsAndPresence() (+53 more)

### Community 11 - "RoomManager"
Cohesion: 0.09
Nodes (6): omnicast/internal/api.WebhookDispatcher, webrtc.API, webrtc.TrackLocalStaticRTP, RoomInfo, RoomManager, RoomSummary

### Community 13 - "🚀 OmniCast Engine — Enterprise Go WebRTC SFU Media Server"
Cohesion: 0.05
Nodes (36): 🌐 100k+ ডিস্ট্রিবিউটেড ক্লাস্টারিং ও লোড ব্যালেন্সিং (Horizontal Scaling), ⚡ 10k+ ভিউয়ার স্কেলিং ও অপ্টিমাইজেশন (Extreme Performance Optimizations), 1. আল্ট্রা-লো লেটেন্সি WebRTC SFU মিডিয়া ইঞ্জিন, 2. মাল্টি-গেস্ট / কো-হোস্টিং সিট ম্যানেজমেন্ট, 3. লাইভ পিকে ব্যাটল ইঞ্জিন (Live PK Battles), 4. গিফট ইকোনমি ও স্কোর সিস্টেম, 5. ডাইনামিক ইউজার মেটাডাটা (No Backend Code Change Needed), 6. ভিডিও বনাম অডিও-অনলি রুম সিকিউরিটি ফিল্টারিং (+28 more)

### Community 14 - "sync.Mutex"
Cohesion: 0.24
Nodes (9): sync.Mutex, GenerateAuthKeyWithSecret(), NewEmbeddedTURNServer(), TestEmbeddedTURNServer_InitializationAndCredentials(), ValidateAndGenerateAuthKey(), ValidateEphemeralCredential(), turn.Server, EmbeddedTURNConfig (+1 more)

### Community 15 - "PacketBuffer"
Cohesion: 0.09
Nodes (16): context.CancelFunc, github.com/pion/rtp.Packet, net.UDPAddr, net.UDPConn, sync.Pool, NewPacketBuffer(), TestPacketBuffer_BasicOperations(), TestPacketBuffer_CircularOverflow() (+8 more)

### Community 16 - "LiveRoomClient"
Cohesion: 0.06
Nodes (8): LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager, PKSession, PublishOptions, RoomStateSnapshot, SDKConfig

### Community 17 - "main"
Cohesion: 0.11
Nodes (25): getEnv(), main(), CascadingYAML, CoHostingYAML, Config, InteractionsYAML, ModerationYAML, PKBattleYAML (+17 more)

### Community 19 - "FanOutDispatcher"
Cohesion: 0.16
Nodes (12): github.com/pion/rtp.Header, webrtc.TrackLocalStaticRTP, NewFanOutDispatcher(), NewSubscriber(), TestFanOutDispatcher_SelectiveFiltering(), TestFanOutDispatcher_SubscribeAndBroadcast(), TestFanOutDispatcher_Unsubscribe(), TestSharedPacket_RetainAndRelease() (+4 more)

### Community 20 - "sync.RWMutex"
Cohesion: 0.06
Nodes (13): sync.RWMutex, NewABRController(), NewDynacastEngine(), TestABRController_Evaluation(), TestDynacastEngine_SubscriberTracking(), NewViewportManager(), TestViewportManager_DefaultsAndVisibility(), TestViewportManager_ResetAndRemove() (+5 more)

### Community 21 - "entrypoint.sh"
Cohesion: 0.40
Nodes (4): REDIS_ADDR, entrypoint.sh script, TURN_REALM, TURN_SECRET

### Community 22 - "PKManager"
Cohesion: 0.23
Nodes (4): PKSession, RoomManager, NewPKManager(), PKManager

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
Nodes (42): AuthHandler, ICEServerJSON, TokenRequest, TokenResponse, UserClaims, fiber.Ctx, github.com/pion/rtcp.NackPair, github.com/pion/rtcp.ReceiverReport (+34 more)

### Community 35 - "turn_auth.go"
Cohesion: 0.33
Nodes (7): GenerateTURNCredentials(), GetDefaultICEServers(), GetDefaultICEServersJSON(), TestGenerateTURNCredentials(), TestGetDefaultICEServers(), webrtc.ICEServer, ICEServerJSON

## Knowledge Gaps
- **39 isolated node(s):** `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET`, `TURN_REALM`, `omnicast` (+34 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `Client`, `TimestampAdjuster`, `RedisBroker`, `testing.T`, `RoomManager`, `sync.Mutex`, `PacketBuffer`, `sync.RWMutex`, `syncRoomStateInternal`, `TestMetricsTracking`, `time.Duration`?**
  _High betweenness centrality (0.207) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `Client`, `RedisBroker`, `testing.T`, `sync.Mutex`, `sync.RWMutex`, `PKManager`, `WorkerPool`, `syncRoomStateInternal`, `TestMetricsTracking`?**
  _High betweenness centrality (0.112) - this node is a cross-community bridge._
- **Why does `TrackSwitcher` connect `TrackSwitcher` to `TimestampAdjuster`, `testing.T`, `PacketBuffer`, `sync.RWMutex`, `WorkerPool`, `time.Duration`?**
  _High betweenness centrality (0.066) - this node is a cross-community bridge._
- **What connects `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET` to the rest of the system?**
  _39 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.052313586796345415 - nodes in this community are weakly interconnected._
- **Should `Client` be split into smaller, more focused modules?**
  _Cohesion score 0.055379746835443035 - nodes in this community are weakly interconnected._
- **Should `TimestampAdjuster` be split into smaller, more focused modules?**
  _Cohesion score 0.06292517006802721 - nodes in this community are weakly interconnected._