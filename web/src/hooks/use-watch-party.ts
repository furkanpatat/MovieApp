import { useCallback, useEffect, useRef, useState } from "react";

import { env } from "@/lib/env";
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
 * Connects to the Watch-Party room for a movie, one connection per hook
 * instance. Deliberately manual, not auto-reconnecting: a dropped connection
 * leaves `status` at "closed"/"error" and the caller re-invokes `connect()`
 * (e.g. the same "Join" button) rather than the hook retrying on its own —
 * simplest thing that can't turn into a reconnect storm.
 *
 * Browsers cannot set an Authorization header on a WebSocket handshake, but
 * they do send cookies: the HttpOnly session cookie authenticates it, and the
 * Gateway checks the handshake's Origin so no other site can ride on it (see
 * services/gateway/internal/server/server.go). The JWT never appears in a URL.
 */
export function useWatchParty(movieId: number, isAuthed: boolean) {
  const [status, setStatus] = useState<ConnectionStatus>("idle");
  const [feed, setFeed] = useState<FeedItem[]>([]);
  const [participantCount, setParticipantCount] = useState(0);
  const [playback, setPlayback] = useState<PlaybackState | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

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
    const room = `movie-${movieId}`;
    const ws = new WebSocket(wsUrl(`/api/v1/watch-party/rooms/${room}/ws`));
    socketRef.current = ws;

    ws.onopen = () => setStatus("open");

    ws.onmessage = (raw) => {
      let event: ServerEvent;
      try {
        event = JSON.parse(raw.data);
      } catch {
        return;
      }

      switch (event.type) {
        case "room_state":
          setParticipantCount(event.participants.length);
          setPlayback(event.playback);
          break;
        case "chat_message":
          pushFeed({ kind: "chat", id: feedId(), userId: event.user_id, text: event.text, at: event.sent_at });
          break;
        case "playback_sync":
          setPlayback({ action: event.action, timestamp: event.timestamp, user_id: event.user_id, updated_at: event.sent_at });
          break;
        case "user_joined":
          setParticipantCount((n) => n + 1);
          pushFeed({ kind: "system", id: feedId(), note: "joined", userId: event.user_id, at: event.sent_at });
          break;
        case "user_left":
          setParticipantCount((n) => Math.max(0, n - 1));
          pushFeed({ kind: "system", id: feedId(), note: "left", userId: event.user_id, at: event.sent_at });
          break;
        case "error":
          pushFeed({ kind: "system", id: feedId(), note: "error", text: event.message, at: new Date().toISOString() });
          break;
      }
    };

    ws.onerror = () => setStatus("error");
    ws.onclose = () => {
      setStatus((s) => (s === "error" ? s : "closed"));
      socketRef.current = null;
    };
  }, [movieId, isAuthed, pushFeed]);

  const disconnect = useCallback(() => {
    socketRef.current?.close(1000, "user left");
    socketRef.current = null;
    setStatus("closed");
  }, []);

  // Always close the socket when the component using this hook unmounts
  // (e.g. navigating away from the movie page) — a leaked open connection
  // would otherwise keep the client, and the room, alive server-side.
  useEffect(() => () => void socketRef.current?.close(1000, "unmounted"), []);

  const send = useCallback((payload: unknown) => {
    const ws = socketRef.current;
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(payload));
  }, []);

  const sendChat = useCallback((text: string) => send({ type: "chat_message", text }), [send]);
  const sendPlaybackSync = useCallback(
    (action: PlaybackAction, timestamp: number) => send({ type: "playback_sync", action, timestamp }),
    [send],
  );

  return { status, feed, participantCount, playback, connect, disconnect, sendChat, sendPlaybackSync };
}
