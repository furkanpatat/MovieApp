"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2, Pause, Play, RotateCw } from "lucide-react";

import { useT } from "@/i18n";
import { displayName } from "@/lib/format";
import type { PlaybackState } from "@/types/watch-party";

// --- YouTube IFrame Player API (loaded once, on demand) ---------------------

interface YTPlayer {
  playVideo(): void;
  pauseVideo(): void;
  seekTo(seconds: number, allowSeekAhead: boolean): void;
  getCurrentTime(): number;
  getDuration(): number;
  getPlayerState(): number;
  destroy(): void;
}

declare global {
  interface Window {
    YT?: { Player: new (el: HTMLElement, options: object) => YTPlayer };
    onYouTubeIframeAPIReady?: () => void;
  }
}

const PLAYING = 1;
const PAUSED = 2;
const BUFFERING = 3;

let apiReady: Promise<void> | null = null;
function loadYouTubeApi(): Promise<void> {
  if (window.YT?.Player) return Promise.resolve();
  apiReady ??= new Promise((resolve) => {
    const previous = window.onYouTubeIframeAPIReady;
    window.onYouTubeIframeAPIReady = () => {
      previous?.();
      resolve();
    };
    const script = document.createElement("script");
    script.src = "https://www.youtube.com/iframe_api";
    script.async = true;
    document.head.appendChild(script);
  });
  return apiReady;
}

function formatTime(total: number): string {
  const s = Math.max(0, Math.floor(total));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/** Whole seconds since an ISO time; 0 for a bad or future one (clock skew). */
function secondsSince(iso: string): number {
  const ms = Date.now() - Date.parse(iso);
  return Number.isFinite(ms) && ms > 0 ? ms / 1000 : 0;
}

/**
 * The Watch Party screen: the movie's YouTube trailer, played in sync with
 * the room. Our own controls (YouTube's are hidden) are the only source of
 * sync events, so applying someone else's play/pause never echoes back.
 * Every event carries the position: play/pause at a time (a skip or a seek
 * sends the current state at the new time), so a late joiner can always
 * catch up; a room that's playing is caught up by the time since its event.
 *
 * Browsers (phones especially) may refuse to start a video that no tap
 * started: a "tap to sync" button then does it with a real gesture.
 */
export function SyncedTrailerPlayer({
  videoKey,
  title,
  playback,
  currentUserId,
  connected,
  onPlay,
  onPause,
}: {
  videoKey: string;
  title: string;
  playback: PlaybackState | null;
  currentUserId: string | null;
  connected: boolean;
  onPlay: (position: number) => void;
  onPause: (position: number) => void;
}) {
  const { t, locale } = useT();
  const hostRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<YTPlayer | null>(null);
  const [ready, setReady] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [blocked, setBlocked] = useState(false);
  // The room's latest state, applied once the player is ready.
  const roomState = useRef<PlaybackState | null>(null);

  // Where the room is now, given its last event.
  const targetOf = (pb: PlaybackState) => (pb.action === "pause" ? pb.timestamp : pb.timestamp + secondsSince(pb.updated_at));

  const apply = useCallback((pb: PlaybackState) => {
    const p = playerRef.current;
    if (!p) return;
    p.seekTo(targetOf(pb), true);
    if (pb.action === "pause") {
      p.pauseVideo(); // (onStateChange clears a "tap to sync" prompt)
      return;
    }
    p.playVideo();
    // Autoplay refused (no tap on this device yet)? Offer one.
    setTimeout(() => {
      if (roomState.current?.action === "pause") return; // paused meanwhile
      const state = playerRef.current?.getPlayerState();
      setBlocked(state !== PLAYING && state !== BUFFERING);
    }, 1500);
  }, []);

  // Create the player (inside a node React doesn't own: YouTube replaces it).
  useEffect(() => {
    let cancelled = false;
    const host = hostRef.current;
    void loadYouTubeApi().then(() => {
      if (cancelled || !host || !window.YT) return;
      const mount = document.createElement("div");
      host.appendChild(mount);
      const player = new window.YT.Player(mount, {
        videoId: videoKey,
        host: "https://www.youtube-nocookie.com",
        width: "100%",
        height: "100%",
        playerVars: { controls: 0, disablekb: 1, fs: 0, rel: 0, playsinline: 1, modestbranding: 1, iv_load_policy: 3 },
        events: {
          onReady: () => {
            if (cancelled) return;
            playerRef.current = player;
            setReady(true);
            setDuration(player.getDuration());
            if (roomState.current) apply(roomState.current);
          },
          onStateChange: (e: { data: number }) => {
            const on = e.data === PLAYING || e.data === BUFFERING;
            setPlaying(on);
            // Playing, or paused by the room: no need to prompt a tap.
            if (on || e.data === PAUSED) setBlocked(false);
          },
        },
      });
    });
    return () => {
      cancelled = true;
      playerRef.current?.destroy();
      playerRef.current = null;
      if (host) host.innerHTML = "";
    };
  }, [videoKey, apply]);

  // Someone else played/paused/skipped (or we just joined): follow.
  useEffect(() => {
    if (!playback) return;
    roomState.current = playback;
    apply(playback);
  }, [playback, apply]);

  // The clock and the progress bar.
  useEffect(() => {
    if (!ready) return;
    const id = setInterval(() => {
      const p = playerRef.current;
      if (!p) return;
      setTime(p.getCurrentTime());
      setDuration(p.getDuration());
    }, 500);
    return () => clearInterval(id);
  }, [ready]);

  const now = () => playerRef.current?.getCurrentTime() ?? 0;

  const toggle = () => {
    const p = playerRef.current;
    if (!p) return;
    if (playing) {
      p.pauseVideo();
      onPause(now());
    } else {
      p.playVideo();
      onPlay(now());
    }
  };

  // Jump to a position, keeping the current play/pause state (for everyone).
  const jumpTo = (position: number) => {
    const p = playerRef.current;
    if (!p) return;
    const to = Math.max(0, Math.min(position, duration || position));
    p.seekTo(to, true);
    setTime(to);
    (playing ? onPlay : onPause)(to);
  };

  const syncByTap = () => {
    const p = playerRef.current;
    const pb = roomState.current;
    if (!p || !pb) return;
    p.seekTo(targetOf(pb), true);
    if (pb.action !== "pause") p.playVideo(); // a real gesture now: allowed
    setBlocked(false);
  };

  const label = !connected
    ? t("player.notSynced")
    : playback
      ? t(playback.action === "play" ? "player.pressedPlay" : "player.paused", {
          name: displayName(playback.user_id, currentUserId, locale),
          time: formatTime(playback.timestamp),
        })
      : t("player.noActivity");

  return (
    <div className="relative aspect-video w-full overflow-hidden rounded-xl bg-black shadow-xl shadow-black/30">
      <div ref={hostRef} className="absolute inset-0 [&>iframe]:size-full" aria-label={t("detail.trailerOf", { title })} />
      {/* Catches clicks on the video (YouTube would toggle it unsynced). */}
      <button type="button" className="absolute inset-0 cursor-pointer" onClick={toggle} aria-label={t(playing ? "player.pause" : "player.play")} />

      {!ready && (
        <div className="absolute inset-0 flex items-center justify-center gap-2 bg-black text-sm text-muted-foreground">
          <Loader2 className="size-5 animate-spin text-primary" />
          {t("player.loading")}
        </div>
      )}

      {blocked && (
        <button
          type="button"
          onClick={syncByTap}
          className="absolute inset-0 z-10 flex items-center justify-center bg-black/60 backdrop-blur-sm"
        >
          <span className="flex items-center gap-2 rounded-full bg-primary px-5 py-3 text-sm font-semibold text-primary-foreground shadow-lg">
            <Play className="size-4 fill-current" />
            {t("player.tapToSync")}
          </span>
        </button>
      )}

      {/* Controls: play/pause, +10s, and a seek bar. */}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/40 to-transparent px-3 pt-10 pb-3">
        <input
          type="range"
          min={0}
          max={Math.max(1, Math.floor(duration))}
          step={1}
          value={Math.min(Math.floor(time), Math.floor(duration) || 0)}
          onChange={(e) => jumpTo(Number(e.target.value))}
          aria-label={t("player.seek")}
          disabled={!ready}
          className="pointer-events-auto h-1 w-full cursor-pointer accent-primary"
        />
        <div className="mt-2 flex items-center gap-2">
          <button
            type="button"
            onClick={toggle}
            disabled={!ready}
            aria-label={t(playing ? "player.pause" : "player.play")}
            className="pointer-events-auto flex size-10 items-center justify-center rounded-full bg-primary text-primary-foreground transition-transform hover:scale-105 active:scale-95 disabled:opacity-50"
          >
            {playing ? <Pause className="size-5 fill-current" /> : <Play className="ml-0.5 size-5 fill-current" />}
          </button>
          <button
            type="button"
            onClick={() => jumpTo(now() + 10)}
            disabled={!ready}
            aria-label={t("player.skip")}
            className="pointer-events-auto flex size-9 items-center justify-center rounded-full bg-white/10 text-white backdrop-blur-sm transition-colors hover:bg-white/20 disabled:opacity-50"
          >
            <RotateCw className="size-4" />
          </button>
          <span className="text-xs font-medium text-zinc-200 tabular-nums">
            {formatTime(time)} / {formatTime(duration)}
          </span>
          <span className="ml-auto truncate rounded-full bg-black/60 px-3 py-1 text-[11px] text-zinc-300">{label}</span>
        </div>
      </div>
    </div>
  );
}
