import { useCallback, useEffect, useRef, useState } from "react";

import { env } from "@/lib/env";
import { useAuthStore } from "@/store/auth-store";
import type {
  ConnectionStatus,
  FeedItem,
  PlaybackAction,
  PlaybackState,
  ServerEvent,
} from "@/types/watch-party";

const MAX_FEED_ITEMS = 300;

function wsUrl(path: string): string {
  return env.apiUrl.replace(/^http/, "ws") + path;
}

function feedId(): string {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random()}`;
}

/**
 * Connects to a Watch-Party room (see lib/party.ts for how rooms are
 * named), one connection per hook instance. Changing `room` leaves the old
 * one: the hook goes back to "idle" until `connect()` joins the new one. Deliberately manual, not auto-reconnecting: a dropped connection
 * leaves `status` at "closed"/"error" and the caller re-invokes `connect()`
 * (e.g. the same "Join" button) rather than the hook retrying on its own —
 * simplest thing that can't turn into a reconnect storm.
 *
 * Browsers cannot set an Authorization header on a WebSocket handshake, but
 * they do send cookies: the HttpOnly session cookie authenticates it, and the
 * Gateway checks the handshake's Origin so no other site can ride on it (see
 * services/gateway/internal/server/server.go). The JWT never appears in a URL.
 */
export function useWatchParty(room: string, isAuthed: boolean) {
  const userId = useAuthStore((s) => s.userId);
  const [status, setStatus] = useState<ConnectionStatus>("idle");
  const [feed, setFeed] = useState<FeedItem[]>([]);
  // Connections per user in the room (a user may have several: two tabs,
  // or a reconnect whose old socket is still being dropped).
  const [members, setMembers] = useState<Record<string, number>>({});
  const [playback, setPlayback] = useState<PlaybackState | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  // A new room starts from scratch (adjusting state during render, as React
  // recommends for "reset when a prop changes"); the effect below closes
  // the old room's socket.
  const [currentRoom, setCurrentRoom] = useState(room);
  if (room !== currentRoom) {
    setCurrentRoom(room);
    setStatus("idle");
    setFeed([]);
    setMembers({});
    setPlayback(null);
  }
  useEffect(
    () => () => {
      const ws = socketRef.current;
      socketRef.current = null; // its events are stale from here on
      if (ws) {
        ws.close(1000, "left the room");
        setStatus("idle"); // not "connecting" forever: connect() may run again
      }
    },
    [room],
  );

  const pushFeed = useCallback((item: FeedItem) => {
    setFeed((prev) => {
      const next = [...prev, item];
      return next.length > MAX_FEED_ITEMS ? next.slice(next.length - MAX_FEED_ITEMS) : next;
    });
  }, []);

  const connect = useCallback(() => {
    if (!isAuthed) {
      setStatus("error");
      return;
    }
    if (socketRef.current && socketRef.current.readyState <= WebSocket.OPEN) {
      return; // already connecting/connected
    }

    setStatus("connecting");
    const ws = new WebSocket(wsUrl(`/api/v1/watch-party/rooms/${room}/ws`));
    socketRef.current = ws;
    // Events from a socket we've since replaced (another room) are stale.
    const current = () => socketRef.current === ws;

    ws.onopen = () => current() && setStatus("open");

    ws.onmessage = (raw) => {
      if (!current()) return;
      let event: ServerEvent;
      try {
        event = JSON.parse(raw.data);
      } catch {
        return;
      }

      switch (event.type) {
        case "room_state": {
          const counts: Record<string, number> = {};
          for (const id of event.participants) counts[id] = (counts[id] ?? 0) + 1;
          setMembers(counts);
          setPlayback(event.playback);
          break;
        }
        case "chat_message":
          pushFeed({ kind: "chat", id: feedId(), userId: event.user_id, text: event.text, at: event.sent_at });
          break;
        case "playback_sync":
          setPlayback({ action: event.action, timestamp: event.timestamp, user_id: event.user_id, updated_at: event.sent_at });
          break;
        case "user_joined":
          setMembers((m) => ({ ...m, [event.user_id]: (m[event.user_id] ?? 0) + 1 }));
          // Your own other connections coming and going aren't news.
          if (event.user_id !== userId) pushFeed({ kind: "system", id: feedId(), note: "joined", userId: event.user_id, at: event.sent_at });
          break;
        case "user_left":
          setMembers((m) => ({ ...m, [event.user_id]: Math.max(0, (m[event.user_id] ?? 0) - 1) }));
          if (event.user_id !== userId) pushFeed({ kind: "system", id: feedId(), note: "left", userId: event.user_id, at: event.sent_at });
          break;
        case "error":
          pushFeed({ kind: "system", id: feedId(), note: "error", text: event.message, at: new Date().toISOString() });
          break;
      }
    };

    ws.onerror = () => current() && setStatus("error");
    ws.onclose = () => {
      if (!current()) return;
      setStatus((s) => (s === "error" ? s : "closed"));
      socketRef.current = null;
    };
  }, [room, isAuthed, pushFeed, userId]);

  const disconnect = useCallback(() => {
    socketRef.current?.close(1000, "user left");
    socketRef.current = null;
    setStatus("closed");
  }, []);

  // The [room] effect above also closes the socket on unmount (e.g.
  // navigating away): a leaked connection would keep the room alive.

  const send = useCallback((payload: unknown) => {
    const ws = socketRef.current;
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(payload));
  }, []);

  // The hub relays a message to everyone but its sender, so your own lines
  // are added here (only if the socket is open, i.e. it was actually sent).
  const sendChat = useCallback(
    (text: string) => {
      if (socketRef.current?.readyState !== WebSocket.OPEN) return;
      send({ type: "chat_message", text });
      if (userId) pushFeed({ kind: "chat", id: feedId(), userId, text, at: new Date().toISOString() });
    },
    [send, pushFeed, userId],
  );
  const sendPlaybackSync = useCallback(
    (action: PlaybackAction, timestamp: number) => send({ type: "playback_sync", action, timestamp }),
    [send],
  );

  // People, not connections; you count while connected even if one of your
  // other sockets just left.
  const participantCount = new Set([
    ...Object.keys(members).filter((id) => members[id] > 0),
    ...(status === "open" && userId ? [userId] : []),
  ]).size;

  return { status, feed, participantCount, playback, connect, disconnect, sendChat, sendPlaybackSync };
}
