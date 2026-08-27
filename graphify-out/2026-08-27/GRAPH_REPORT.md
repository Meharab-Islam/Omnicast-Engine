# Graph Report - go_media_server  (2026-08-27)

## Corpus Check
- 48 files · ~57,128 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 553 nodes · 1103 edges · 20 communities (15 shown, 5 thin omitted)
- Extraction: 96% EXTRACTED · 4% INFERRED · 0% AMBIGUOUS · INFERRED: 49 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `e91e5390`
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
- models.go
- RoomManager
- api/auth.go
- 🚀 Go Live Media Server (SFU & Interactive Streaming)
- CascadeManager
- turn_auth.go
- LiveRoomClient
- main
- omnicast
- entrypoint.sh

## God Nodes (most connected - your core abstractions)
1. `Room` - 86 edges
2. `RoomManager` - 46 edges
3. `RedisBroker` - 40 edges
4. `Client` - 40 edges
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

## Communities (20 total, 5 thin omitted)

### Community 0 - "Room"
Cohesion: 0.09
Nodes (6): time.Duration, time.Timer, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, CoHostMedia

### Community 1 - "Client"
Cohesion: 0.06
Nodes (28): encoding/json.RawMessage, github.com/gofiber/contrib/websocket.Conn, GetAppConfig(), SignalingMessage, RoomManager, webrtc.API, webrtc.PeerConnection, NewClient() (+20 more)

### Community 2 - "testing.T"
Cohesion: 0.09
Nodes (28): testing.T, TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations(), ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode(), NewRoom(), NewRoomWithName() (+20 more)

### Community 3 - "RedisBroker"
Cohesion: 0.12
Nodes (10): MessageHandler, context.Context, time.Time, RedisBroker, NewRedisBroker(), PKSession, MediaState, RoomState (+2 more)

### Community 5 - "WebhookDispatcher"
Cohesion: 0.12
Nodes (12): WebhookEvent, WebhookEventType, net/http.Client, sync.Once, sync.WaitGroup, GenerateSignature(), WebhookDispatcher, NewWebhookDispatcher() (+4 more)

### Community 6 - "LiveRoomClient"
Cohesion: 0.06
Nodes (5): EventEmitter, LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager

### Community 7 - "TrackSwitcher"
Cohesion: 0.08
Nodes (18): github.com/pion/rtp.Packet, sync.RWMutex, NewABRController(), NewDynacastEngine(), TestABRController_Evaluation(), TestDynacastEngine_SubscriberTracking(), webrtc.TrackLocalStaticRTP, NewTrackSwitcher() (+10 more)

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
Cohesion: 0.17
Nodes (8): context.CancelFunc, github.com/fasthttp/websocket.Conn, sync.Mutex, webrtc.API, webrtc.PeerConnection, NewCascadeManager(), CascadeManager, CascadeSession

### Community 15 - "turn_auth.go"
Cohesion: 0.33
Nodes (7): GenerateTURNCredentials(), GetDefaultICEServers(), GetDefaultICEServersJSON(), TestGenerateTURNCredentials(), TestGetDefaultICEServers(), webrtc.ICEServer, ICEServerJSON

### Community 16 - "LiveRoomClient"
Cohesion: 0.06
Nodes (8): LiveMediaManager, LiveMediaSDK, LiveRoomClient, LiveStateManager, PKSession, PublishOptions, RoomStateSnapshot, SDKConfig

### Community 17 - "main"
Cohesion: 0.12
Nodes (25): getEnv(), main(), CascadingYAML, CoHostingYAML, Config, InteractionsYAML, ModerationYAML, PKBattleYAML (+17 more)

### Community 21 - "entrypoint.sh"
Cohesion: 0.40
Nodes (4): REDIS_ADDR, entrypoint.sh script, TURN_REALM, TURN_SECRET

## Knowledge Gaps
- **29 isolated node(s):** `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET`, `TURN_REALM`, `omnicast` (+24 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `Client`, `testing.T`, `RedisBroker`, `TrackSwitcher`, `RoomManager`, `CascadeManager`?**
  _High betweenness centrality (0.194) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `Client`, `testing.T`, `RedisBroker`, `WebhookDispatcher`, `TrackSwitcher`, `CascadeManager`?**
  _High betweenness centrality (0.141) - this node is a cross-community bridge._
- **Why does `RedisBroker` connect `RedisBroker` to `RoomManager`, `CascadeManager`, `TrackSwitcher`?**
  _High betweenness centrality (0.072) - this node is a cross-community bridge._
- **What connects `entrypoint.sh script`, `REDIS_ADDR`, `TURN_SECRET` to the rest of the system?**
  _29 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.08651911468812877 - nodes in this community are weakly interconnected._
- **Should `Client` be split into smaller, more focused modules?**
  _Cohesion score 0.06351236146632566 - nodes in this community are weakly interconnected._
- **Should `testing.T` be split into smaller, more focused modules?**
  _Cohesion score 0.08906882591093117 - nodes in this community are weakly interconnected._