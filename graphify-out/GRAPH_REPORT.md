# Graph Report - go_media_server  (2026-08-27)

## Corpus Check
- 47 files · ~54,845 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 540 nodes · 1075 edges · 20 communities (14 shown, 6 thin omitted)
- Extraction: 95% EXTRACTED · 5% INFERRED · 0% AMBIGUOUS · INFERRED: 49 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `67287386`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Room
- Client
- testing.T
- RedisBroker
- LiveMediaCore
- WebhookDispatcher
- .emit
- sync.RWMutex
- signaling.go
- webrtc.go
- models.go
- RoomManager
- api/auth.go
- 🚀 Go Live Media Server (SFU & Interactive Streaming)
- CascadeManager
- HandleCoHostConnection
- LiveRoomClient
- main
- live-media-server
- LiveRoomClient

## God Nodes (most connected - your core abstractions)
1. `Room` - 83 edges
2. `RoomManager` - 46 edges
3. `RedisBroker` - 40 edges
4. `Client` - 36 edges
5. `SignalingMessage` - 29 edges
6. `LiveMediaCore` - 25 edges
7. `LiveRoomClient` - 17 edges
8. `main()` - 15 edges
9. `NewRoomManager()` - 15 edges
10. `WebhookDispatcher` - 14 edges

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

## Communities (20 total, 6 thin omitted)

### Community 0 - "Room"
Cohesion: 0.09
Nodes (6): time.Duration, time.Timer, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, CoHostMedia

### Community 1 - "Client"
Cohesion: 0.08
Nodes (21): encoding/json.RawMessage, github.com/gofiber/contrib/websocket.Conn, GetAppConfig(), SignalingMessage, ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode(), RoomManager (+13 more)

### Community 2 - "testing.T"
Cohesion: 0.08
Nodes (32): testing.T, TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations(), NewRoom(), NewRoomWithName(), TestRoomCoHostTracks(), TestRoomSimulcastTracks(), TestRoomState() (+24 more)

### Community 3 - "RedisBroker"
Cohesion: 0.12
Nodes (10): MessageHandler, context.Context, time.Time, RedisBroker, NewRedisBroker(), PKSession, MediaState, RoomState (+2 more)

### Community 5 - "WebhookDispatcher"
Cohesion: 0.12
Nodes (12): WebhookEvent, WebhookEventType, net/http.Client, sync.Once, sync.WaitGroup, GenerateSignature(), WebhookDispatcher, NewWebhookDispatcher() (+4 more)

### Community 6 - ".emit"
Cohesion: 0.08
Nodes (4): EventEmitter, LiveMediaManager, LiveMediaSDK, LiveStateManager

### Community 7 - "sync.RWMutex"
Cohesion: 0.09
Nodes (12): github.com/pion/rtp.Packet, sync.RWMutex, NewABRController(), NewDynacastEngine(), TestABRController_Evaluation(), TestDynacastEngine_SubscriberTracking(), webrtc.TrackLocalStaticRTP, NewTrackSwitcher() (+4 more)

### Community 11 - "RoomManager"
Cohesion: 0.07
Nodes (8): github.com/pion/webrtc/v3.TrackLocalStaticRTP, RoomManager, NewPKManager(), syncRoomStateInternal(), PKManager, RoomInfo, RoomManager, RoomSummary

### Community 12 - "api/auth.go"
Cohesion: 0.19
Nodes (10): AuthHandler, ICEServerJSON, TokenRequest, TokenResponse, UserClaims, fiber.Ctx, GenerateUserToken(), jwt.RegisteredClaims (+2 more)

### Community 13 - "🚀 Go Live Media Server (SFU & Interactive Streaming)"
Cohesion: 0.08
Nodes (24): 1. Flutter (Dart) SDK Implementation, 2. React Native / TypeScript SDK Implementation, 3. iOS (Swift) SDK Implementation, 4. Android (Kotlin) SDK Implementation, 🚀 Go Live Media Server (SFU & Interactive Streaming), 🌐 REST API Endpoints, 🏗️ আর্কিটেকচার ওভারভিউ (Architecture Overview), ইভেন্ট রেফারেন্স টেবিল: (+16 more)

### Community 14 - "CascadeManager"
Cohesion: 0.18
Nodes (8): context.CancelFunc, github.com/fasthttp/websocket.Conn, sync.Mutex, webrtc.API, webrtc.PeerConnection, NewCascadeManager(), CascadeManager, CascadeSession

### Community 15 - "HandleCoHostConnection"
Cohesion: 0.19
Nodes (16): webrtc.API, webrtc.Configuration, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, HandleCoHostConnection(), HandleHostConnection(), GetRTPBuffer(), PutRTPBuffer() (+8 more)

### Community 16 - "LiveRoomClient"
Cohesion: 0.06
Nodes (8): LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager, PKSession, PublishOptions, RoomStateSnapshot, SDKConfig

### Community 17 - "main"
Cohesion: 0.12
Nodes (25): getEnv(), main(), CascadingYAML, CoHostingYAML, Config, InteractionsYAML, ModerationYAML, PKBattleYAML (+17 more)

## Knowledge Gaps
- **25 isolated node(s):** `live-media-server`, `TokenRequest`, `Participant`, `SDKConfig`, `PublishOptions` (+20 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `Client`, `testing.T`, `RedisBroker`, `sync.RWMutex`, `RoomManager`, `CascadeManager`, `HandleCoHostConnection`?**
  _High betweenness centrality (0.194) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `Client`, `testing.T`, `RedisBroker`, `WebhookDispatcher`, `sync.RWMutex`, `CascadeManager`?**
  _High betweenness centrality (0.145) - this node is a cross-community bridge._
- **Why does `RedisBroker` connect `RedisBroker` to `RoomManager`, `CascadeManager`, `sync.RWMutex`?**
  _High betweenness centrality (0.074) - this node is a cross-community bridge._
- **What connects `live-media-server`, `TokenRequest`, `Participant` to the rest of the system?**
  _25 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.08955223880597014 - nodes in this community are weakly interconnected._
- **Should `Client` be split into smaller, more focused modules?**
  _Cohesion score 0.07922077922077922 - nodes in this community are weakly interconnected._
- **Should `testing.T` be split into smaller, more focused modules?**
  _Cohesion score 0.07822410147991543 - nodes in this community are weakly interconnected._