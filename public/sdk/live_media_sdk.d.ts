/**
 * TypeScript Definitions for LiveMediaSDK
 */

export interface SDKConfig {
  hostUrl: string;
  apiKey: string;
  apiSecret: string;
  userId?: string;
  userName?: string;
  avatarUrl?: string;
  wsUrl?: string;
}

export interface PublishOptions {
  stream?: MediaStream;
  video?: boolean;
  audio?: boolean;
  simulcast?: boolean;
}

export interface RoomStateSnapshot {
  roomId: string;
  roomName: string;
  hostId: string;
  mainSeatId: string;
  totalViewers: number;
  viewersList: string[];
  hostScore: number;
  activeSeats: Record<string, string>;
  mediaStates: Record<string, { muted_audio: boolean; muted_video: boolean }>;
  pkSession: PKSession | null;
}

export interface PKSession {
  sessionId?: string;
  room1?: string;
  room2?: string;
  host1?: string;
  host2?: string;
  score1?: number;
  score2?: number;
}

export declare class LiveStateManager {
  roomId: string;
  roomName: string;
  hostId: string;
  totalViewers: number;
  hostScore: number;
  activeSeats: Record<string, string>;
  mediaStates: Record<string, { muted_audio: boolean; muted_video: boolean }>;
  pkSession: PKSession | null;

  on(event: 'onStateSynced', listener: (snapshot: RoomStateSnapshot) => void): this;
  on(event: 'onViewersUpdated', listener: (data: { totalViewers: number; viewersList: string[] }) => void): this;
  on(event: 'onScoreUpdated', listener: (score: number) => void): this;
  on(event: 'onSeatsUpdated', listener: (seats: Record<string, string>) => void): this;
  on(event: 'onMediaStateChanged', listener: (data: { userId: string; mediaState: { muted_audio: boolean; muted_video: boolean } }) => void): this;
  on(event: 'onPKSessionChanged', listener: (session: PKSession | null) => void): this;
  on(event: 'onPKScoreUpdated', listener: (scores: { score1: number; score2: number }) => void): this;
  on(event: string, listener: (...args: any[]) => void): this;

  getSnapshot(): RoomStateSnapshot;
}

export declare class LiveMediaManager {
  peerConnection: RTCPeerConnection | null;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  isPublishing: boolean;
  isAudioMuted: boolean;
  isVideoMuted: boolean;

  on(event: 'onLocalStream', listener: (stream: MediaStream) => void): this;
  on(event: 'onRemoteTrack', listener: (data: { track: MediaStreamTrack; stream: MediaStream; kind: string }) => void): this;
  on(event: 'onIceStateChange', listener: (state: RTCIceConnectionState) => void): this;
  on(event: 'onConnectionStateChange', listener: (state: RTCPeerConnectionState) => void): this;
  on(event: string, listener: (...args: any[]) => void): this;

  publishCamera(options?: PublishOptions): Promise<MediaStream>;
  subscribeViewer(): Promise<void>;
  toggleAudio(mute?: boolean): void;
  toggleVideo(mute?: boolean): void;
  setDynacastLayerActive(rid: 'f' | 'h' | 'q', active: boolean): Promise<void>;
  stop(): void;
}

export declare class LiveRoomClient {
  hostUrl: string;
  roomId: string;
  userId: string;
  userName: string;
  token: string;
  isConnected: boolean;

  on(event: 'onConnected', listener: () => void): this;
  on(event: 'onDisconnected', listener: (event: CloseEvent) => void): this;
  on(event: 'onError', listener: (error: any) => void): this;
  on(event: string, listener: (...args: any[]) => void): this;

  send(action: string, payload?: any): boolean;
  sendChat(text: string): boolean;
  sendGift(giftName: string, coins?: number): boolean;
  requestSeat(seatId?: number): boolean;
  acceptSeat(viewerId: string, seatId?: number): boolean;
  leaveSeat(): boolean;
  kickSeat(targetUserId: string): boolean;
  requestPK(targetRoomId: string): boolean;
  acceptPK(fromRoomId: string): boolean;
  stopPK(targetRoomId: string): boolean;
  disconnect(): void;
}

export declare class LiveMediaSDK {
  room: LiveRoomClient;
  media: LiveMediaManager;
  state: LiveStateManager;

  on(event: string, listener: (...args: any[]) => void): this;
  initialize(config: SDKConfig): Promise<this>;
  createRoom(options?: { roomId?: string; roomName?: string }): Promise<void>;
  joinRoom(roomId: string): Promise<void>;
  destroy(): void;
}

export default LiveMediaSDK;
