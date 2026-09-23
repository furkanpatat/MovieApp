"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { Pause, Play, RotateCw } from "lucide-react";

import { backdropUrl } from "@/lib/tmdb-image";
import { displayName } from "@/lib/format";
import type { PlaybackState } from "@/types/watch-party";

function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const m = Math.floor(s / 60);
  const rem = s % 60;
  return `${m}:${rem.toString().padStart(2, "0")}`;
}

/**
 * A mock player: there is no real video, only a synced position ticking
 * locally, kept in step with whatever the room's other members do via
 * playback_sync. Play/pause here is optimistic (applied to the local clock
 * immediately); the `playback` prop only ever changes from an *incoming*
 * event, since the server never echoes a broadcast back to its sender — so
 * there's no fight between the two paths, just a one-way sync-on-receive.
 */
export function VideoPlayerPlaceholder({
  title,
  backdropPath,
  playback,
  currentUserId,
  connected,
  onPlay,
  onPause,
  onSeek,
}: {
  title: string;
  backdropPath?: string;
  playback: PlaybackState | null;
  currentUserId: string | null;
  connected: boolean;
  onPlay: (position: number) => void;
  onPause: (position: number) => void;
  onSeek: (position: number) => void;
}) {
  const [isPlaying, setIsPlaying] = useState(false);
  const [position, setPosition] = useState(0);
  const [lastEvent, setLastEvent] = useState<PlaybackState | null>(null);
  const tickRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Adopt an incoming sync (another member's action, or the room's state as
  // of when we joined) as the new truth. Done during render — not an effect
  // — per React's guidance for "adjust state when a prop changes": comparing
  // against the previously-seen prop value and setting state conditionally
  // in the render body avoids an extra render pass and the cascading-update
  // lint warning an effect-based version would trigger here.
  const [syncedPlayback, setSyncedPlayback] = useState(playback);
  if (playback !== syncedPlayback) {
    setSyncedPlayback(playback);
    if (playback) {
      setIsPlaying(playback.action !== "pause");
      setPosition(playback.timestamp);
      setLastEvent(playback);
    }
  }

  useEffect(() => {
    if (isPlaying) {
      tickRef.current = setInterval(() => setPosition((p) => p + 1), 1000);
    }
    return () => {
      if (tickRef.current) clearInterval(tickRef.current);
    };
  }, [isPlaying]);

  const backdrop = backdropUrl(backdropPath, "w1280");

  const toggle = () => {
    const next = !isPlaying;
    setIsPlaying(next);
    (next ? onPlay : onPause)(position);
  };

  const skip = () => {
    const next = position + 10;
    setPosition(next);
    onSeek(next);
  };

  return (
    <div className="relative aspect-video w-full overflow-hidden rounded-xl bg-secondary shadow-xl shadow-black/30">
      {backdrop && (
        <Image src={backdrop} alt="" fill sizes="70vw" className="object-cover opacity-40" priority />
      )}
      <div className="absolute inset-0 bg-gradient-to-t from-background via-background/30 to-background/10" />

      <div className="absolute inset-0 flex flex-col items-center justify-center gap-4">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={toggle}
            aria-label={isPlaying ? "Pause" : "Play"}
            className="flex size-16 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg transition-transform hover:scale-105 active:scale-95"
          >
            {isPlaying ? <Pause className="size-7 fill-current" /> : <Play className="ml-1 size-7 fill-current" />}
          </button>
          <button
            type="button"
            onClick={skip}
            aria-label="Skip 10 seconds"
            className="flex size-10 items-center justify-center rounded-full bg-black/50 text-foreground backdrop-blur-sm transition-colors hover:bg-black/70"
          >
            <RotateCw className="size-4" />
          </button>
        </div>
        <p className="text-sm font-medium text-muted-foreground">
          {title} — {formatDuration(position)}
        </p>
      </div>

      <div className="absolute bottom-3 left-3 rounded-full bg-black/60 px-3 py-1 text-xs text-muted-foreground backdrop-blur-sm">
        {!connected
          ? "Not synced — join the watch party to sync playback"
          : lastEvent
            ? `${displayName(lastEvent.user_id, currentUserId)} ${lastEvent.action === "seek" ? "skipped to" : lastEvent.action === "play" ? "pressed play" : "paused"} · ${formatDuration(lastEvent.timestamp)}`
            : "Synced — no activity yet"}
      </div>
    </div>
  );
}
