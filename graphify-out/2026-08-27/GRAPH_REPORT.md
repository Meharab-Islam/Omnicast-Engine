# Graph Report - go_media_server  (2026-08-27)

## Corpus Check
- 28 files · ~24,944 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 264 nodes · 555 edges · 15 communities (10 shown, 5 thin omitted)
- Extraction: 97% EXTRACTED · 3% INFERRED · 0% AMBIGUOUS · INFERRED: 18 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- Room
- Client
- testing.T
- RedisBroker
- LiveMediaCore
- WebhookDispatcher
- PKManager
- HandleCoHostConnection
- signaling.go
- webrtc.go
- models.go
- RoomManager
- main
- HandleViewerConnection
- live-media-server

## God Nodes (most connected - your core abstractions)
1. `Room` - 63 edges
2. `RoomManager` - 29 edges
3. `Client` - 28 edges
4. `RedisBroker` - 24 edges
5. `SignalingMessage` - 19 edges
6. `LiveMediaCore` - 18 edges
7. `WebhookDispatcher` - 14 edges
8. `CascadeManager` - 14 edges
9. `main()` - 12 edges
10. `Hub` - 12 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `NewWebhookDispatcher()`  [EXTRACTED]
  cmd/main.go → internal/api/webhook.go
- `main()` --calls--> `NewRedisBroker()`  [EXTRACTED]
  cmd/main.go → internal/broker/redis_broker.go
- `main()` --calls--> `NewClientWithClaims()`  [EXTRACTED]
  cmd/main.go → internal/signaling/client.go
- `main()` --calls--> `NewHub()`  [EXTRACTED]
  cmd/main.go → internal/signaling/hub.go
- `main()` --calls--> `NewPKManager()`  [EXTRACTED]
  cmd/main.go → internal/signaling/pk_manager.go

## Import Cycles
- None detected.

## Communities (15 total, 5 thin omitted)

### Community 0 - "Room"
Cohesion: 0.12
Nodes (6): time.Timer, Room, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, NewRoomWithName(), CoHostMedia

### Community 1 - "Client"
Cohesion: 0.12
Nodes (13): encoding/json.RawMessage, github.com/gofiber/contrib/websocket.Conn, sync.RWMutex, SignalingMessage, RoomManager, webrtc.API, webrtc.PeerConnection, NewClient() (+5 more)

### Community 2 - "testing.T"
Cohesion: 0.10
Nodes (19): testing.T, TestNewRedisBroker_EmptyAddr(), TestRedisBroker_NilOperations(), ParseSignalingMessage(), TestParseSignalingMessage(), TestSignalingMessageEncode(), NewRoom(), TestRoomCoHostTracks() (+11 more)

### Community 3 - "RedisBroker"
Cohesion: 0.10
Nodes (15): MessageHandler, context.CancelFunc, context.Context, github.com/fasthttp/websocket.Conn, sync.Mutex, time.Duration, RedisBroker, NewRedisBroker() (+7 more)

### Community 5 - "WebhookDispatcher"
Cohesion: 0.18
Nodes (9): WebhookEvent, WebhookEventType, net/http.Client, sync.WaitGroup, GenerateSignature(), WebhookDispatcher, NewWebhookDispatcher(), TestWebhookDispatcher() (+1 more)

### Community 6 - "PKManager"
Cohesion: 0.27
Nodes (7): time.Time, addTracksToPeer(), RoomManager, webrtc.TrackLocalStaticRTP, NewPKManager(), PKManager, PKSession

### Community 7 - "HandleCoHostConnection"
Cohesion: 0.43
Nodes (6): webrtc.API, webrtc.Configuration, webrtc.PeerConnection, webrtc.TrackLocalStaticRTP, HandleCoHostConnection(), HandleHostConnection()

### Community 11 - "RoomManager"
Cohesion: 0.13
Nodes (5): github.com/pion/webrtc/v3.TrackLocalStaticRTP, broadcastToRoomInternal(), RoomInfo, RoomManager, RoomSummary

### Community 12 - "main"
Cohesion: 0.29
Nodes (8): getEnv(), main(), GenerateToken(), GetJWTSecret(), TestJWTAuthentication(), ValidateToken(), jwt.RegisteredClaims, UserClaims

### Community 15 - "HandleViewerConnection"
Cohesion: 0.36
Nodes (8): EstimateViewerBandwidth(), webrtc.API, webrtc.Configuration, webrtc.PeerConnection, HandleViewerConnection(), HandleViewerConnectionForRoom(), RoomManager, webrtc.RTPSender

## Knowledge Gaps
- **2 isolated node(s):** `live-media-server`, `Participant`
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Room` connect `Room` to `Client`, `testing.T`, `RedisBroker`, `PKManager`, `HandleCoHostConnection`, `RoomManager`, `HandleViewerConnection`?**
  _High betweenness centrality (0.356) - this node is a cross-community bridge._
- **Why does `RoomManager` connect `RoomManager` to `Room`, `Client`, `testing.T`, `RedisBroker`, `WebhookDispatcher`?**
  _High betweenness centrality (0.217) - this node is a cross-community bridge._
- **Why does `RedisBroker` connect `RedisBroker` to `Client`, `RoomManager`?**
  _High betweenness centrality (0.105) - this node is a cross-community bridge._
- **What connects `live-media-server`, `Participant` to the rest of the system?**
  _2 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Room` be split into smaller, more focused modules?**
  _Cohesion score 0.11764705882352941 - nodes in this community are weakly interconnected._
- **Should `Client` be split into smaller, more focused modules?**
  _Cohesion score 0.12299465240641712 - nodes in this community are weakly interconnected._
- **Should `testing.T` be split into smaller, more focused modules?**
  _Cohesion score 0.10344827586206896 - nodes in this community are weakly interconnected._