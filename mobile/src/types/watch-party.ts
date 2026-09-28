// Wire types for the Watch-Party WebSocket protocol (same as web/src/types;
// source of truth: services/watchparty/internal/hub/message.go).

export type PlaybackAction = "play" | "pause" | "seek";

export interface PlaybackState {
  action: PlaybackAction;
  timestamp: number;
  user_id: string;
  updated_at: string;
}

interface Base {
  room: string;
  sent_at: string;
}

export type ServerEvent =
  | (Base & { type: "chat_message"; user_id: string; text: string })
  | (Base & { type: "playback_sync"; user_id: string; action: PlaybackAction; timestamp: number })
  | (Base & { type: "user_joined"; user_id: string })
  | (Base & { type: "user_left"; user_id: string })
  | { type: "room_state"; room: string; participants: string[]; playback: PlaybackState | null }
  | { type: "error"; message: string };

export type ConnectionStatus = "idle" | "connecting" | "open" | "closed" | "error";

export type FeedItem =
  | { kind: "chat"; id: string; userId: string; text: string; at: string }
  | { kind: "system"; id: string; at: string; note: "joined" | "left"; userId: string }
  | { kind: "system"; id: string; at: string; note: "error"; text: string };
