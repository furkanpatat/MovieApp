import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "@/lib/api";
import { displayName, SITE } from "@/lib/party";
import { useAuth } from "@/store/auth";
import type { ConnectionStatus, FeedItem, PlaybackAction, PlaybackState, ServerEvent } from "@/types/watch-party";

const MAX_FEED = 300;
let seq = 0;
const feedId = () => `${Date.now()}-${seq++}`;

/**
 * One Watch-Party room over a WebSocket, joined while the screen is open.
 * The app authenticates the handshake with its Bearer token (a browser can
 * only use its cookie). iOS's WebSocket would send the API's own address as
 * Origin, which the party service doesn't allow; the site's is sent instead.
 * No auto-reconnect: a drop shows "Reconnect", the user's call.
 */
export function useWatchParty(room: string) {
  const { token, userId } = useAuth();
  // Joining starts at once (the effect below), so that's the first state.
  const [status, setStatus] = useState<ConnectionStatus>(token ? "connecting" : "error");
  const [feed, setFeed] = useState<FeedItem[]>([]);
  const [members, setMembers] = useState<Record<string, number>>({});
  // The room's latest event (for the status line), and the latest one from
  // someone else: the player follows only those (it already did your own).
  const [lastEvent, setLastEvent] = useState<PlaybackState | null>(null);
  const [playback, setPlayback] = useState<PlaybackState | null>(null);
  const socket = useRef<WebSocket | null>(null);

  const push = useCallback((item: FeedItem) => setFeed((f) => [...f, item].slice(-MAX_FEED)), []);

  // Opens the socket; state changes only in its callbacks.
  const open = useCallback(() => {
    if (!token || (socket.current && socket.current.readyState <= WebSocket.OPEN)) return;
    const url = API_URL.replace(/^http/, "ws") + `/api/v1/watch-party/rooms/${room}/ws`;
    // React Native's WebSocket takes headers as a third argument.
    const ws = new (WebSocket as unknown as new (u: string, p: null, o: { headers: Record<string, string> }) => WebSocket)(url, null, {
      headers: { Authorization: `Bearer ${token}`, Origin: SITE },
    });
    socket.current = ws;
    const current = () => socket.current === ws;

    ws.onopen = () => current() && setStatus("open");
    ws.onmessage = (raw) => {
      if (!current()) return;
      let e: ServerEvent;
      try {
        e = JSON.parse(String(raw.data));
      } catch {
        return;
      }
      switch (e.type) {
        case "room_state": {
          const counts: Record<string, number> = {};
          for (const id of e.participants) counts[id] = (counts[id] ?? 0) + 1;
          setMembers(counts);
          setPlayback(e.playback);
          setLastEvent(e.playback);
          break;
        }
        case "chat_message":
          push({ kind: "chat", id: feedId(), userId: e.user_id, text: e.text, at: e.sent_at });
          break;
        case "playback_sync": {
          const pb = { action: e.action, timestamp: e.timestamp, user_id: e.user_id, updated_at: e.sent_at };
          setPlayback(pb);
          setLastEvent(pb);
          break;
        }
        case "user_joined":
          setMembers((m) => ({ ...m, [e.user_id]: (m[e.user_id] ?? 0) + 1 }));
          if (e.user_id !== userId) push({ kind: "system", id: feedId(), at: e.sent_at, text: `${displayName(e.user_id, userId)} joined` });
          break;
        case "user_left":
          setMembers((m) => ({ ...m, [e.user_id]: Math.max(0, (m[e.user_id] ?? 0) - 1) }));
          if (e.user_id !== userId) push({ kind: "system", id: feedId(), at: e.sent_at, text: `${displayName(e.user_id, userId)} left` });
          break;
        case "error":
          push({ kind: "system", id: feedId(), at: new Date().toISOString(), text: e.message });
          break;
      }
    };
    ws.onerror = () => current() && setStatus("error");
    ws.onclose = () => {
      if (!current()) return;
      setStatus((s) => (s === "error" ? s : "closed"));
      socket.current = null;
    };
  }, [room, token, userId, push]);

  /** Reconnect (the user's button). */
  const connect = useCallback(() => {
    if (!token) return setStatus("error");
    setStatus("connecting");
    open();
  }, [token, open]);

  // Join on open; leave (close the socket) on the way out.
  useEffect(() => {
    open();
    return () => {
      const ws = socket.current;
      socket.current = null;
      ws?.close(1000, "left the room");
    };
  }, [open]);

  const send = useCallback((payload: unknown) => {
    const ws = socket.current;
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(payload));
    return ws?.readyState === WebSocket.OPEN;
  }, []);

  // The hub relays to everyone but the sender: your own lines are added here.
  const sendChat = useCallback(
    (text: string) => {
      if (send({ type: "chat_message", text }) && userId) push({ kind: "chat", id: feedId(), userId, text, at: new Date().toISOString() });
    },
    [send, push, userId],
  );
  const sendPlayback = useCallback(
    (action: PlaybackAction, timestamp: number) => {
      if (send({ type: "playback_sync", action, timestamp }) && userId) {
        setLastEvent({ action, timestamp, user_id: userId, updated_at: new Date().toISOString() });
      }
    },
    [send, userId],
  );

  const people = new Set([...Object.keys(members).filter((id) => members[id] > 0), ...(status === "open" && userId ? [userId] : [])]).size;

  return { status, feed, people, playback, lastEvent, connect, sendChat, sendPlayback, me: userId };
}
