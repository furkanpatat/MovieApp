// Wire types for the Watch-Party WebSocket protocol (see
// services/watchparty/internal/hub/message.go for the source of truth).

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

/** One line in the chat stream: an actual message, or a subtle system note
 *  (join/left/error). System notes carry structured data rather than a
 *  pre-formatted string — join/left include the raw user_id, which the
 *  rendering layer turns into "You" or a short display name (see
 *  lib/format.ts's displayName), the same way chat messages already do. */
export type FeedItem =
  | { kind: "chat"; id: string; userId: string; text: string; at: string }
  | { kind: "system"; id: string; at: string; note: "joined" | "left"; userId: string }
  | { kind: "system"; id: string; at: string; note: "error"; text: string };
