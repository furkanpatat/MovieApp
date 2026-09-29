"use client";

import { useEffect, useRef } from "react";

import { BUFFERING, CUED, loadYouTubeApi, PAUSED, PLAYING, UNSTARTED, type YTEvent, type YTPlayer } from "@/lib/youtube";

/** How long a player may take to start before the watchdog restarts it. */
const START_TIMEOUT_MS = 1500;
const MAX_RESTARTS = 3;
/** A pause this soon after turning the sound on is the browser refusing it. */
const UNMUTE_GRACE_MS = 1500;

/**
 * The trailer behind a Discover post, through the YouTube IFrame Player API.
 *
 * Browsers autoplay only muted video without a gesture, and a scroll isn't
 * one. So every player starts muted and plays; sound is turned on only once
 * it is playing. If the browser refuses (it pauses the video, or keeps it
 * muted), the player goes back to muted and plays on, and onSoundBlocked
 * tells the feed to show sound as off: a tap on the mute button is a gesture,
 * and sound works from there. A player that hasn't started after
 * START_TIMEOUT_MS is restarted muted (up to MAX_RESTARTS times), so a post
 * never sits on YouTube's play screen.
 */
export function FeedPlayer({
  videoKey,
  playing,
  muted,
  onSoundBlocked,
}: {
  videoKey: string;
  playing: boolean;
  muted: boolean;
  onSoundBlocked: () => void;
}) {
  const hostRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<YTPlayer | null>(null);
  // What the feed wants right now, read by the player's event handlers.
  const want = useRef({ playing, muted, onSoundBlocked });
  useEffect(() => {
    want.current = { playing, muted, onSoundBlocked };
  });

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    // YT.Player replaces this element with its iframe.
    const el = document.createElement("div");
    host.appendChild(el);

    let cancelled = false;
    let restarts = 0;
    let unmutedAt = 0;
    let watchdog: ReturnType<typeof setTimeout> | undefined;
    let soundCheck: ReturnType<typeof setTimeout> | undefined;

    const soundBlocked = (p: YTPlayer) => {
      unmutedAt = 0;
      p.mute();
      p.playVideo();
      want.current.onSoundBlocked();
    };

    const armWatchdog = (p: YTPlayer) => {
      clearTimeout(watchdog);
      watchdog = setTimeout(() => {
        if (cancelled || !want.current.playing) return;
        const state = p.getPlayerState();
        if (state === PLAYING || state === BUFFERING || restarts >= MAX_RESTARTS) return;
        restarts++;
        p.mute();
        p.playVideo();
        armWatchdog(p);
      }, START_TIMEOUT_MS);
    };

    void loadYouTubeApi().then(() => {
      if (cancelled || !window.YT) return;
      playerRef.current = new window.YT.Player(el, {
        videoId: videoKey,
        width: "100%",
        height: "100%",
        playerVars: {
          autoplay: 1,
          mute: 1,
          controls: 0,
          disablekb: 1,
          fs: 0,
          iv_load_policy: 3,
          modestbranding: 1,
          rel: 0,
          playsinline: 1,
          loop: 1,
          playlist: videoKey,
          origin: window.location.origin,
        },
        events: {
          onReady: (e: YTEvent) => {
            if (cancelled) return;
            e.target.mute();
            if (want.current.playing) e.target.playVideo();
            armWatchdog(e.target);
          },
          onStateChange: (e: YTEvent) => {
            if (cancelled) return;
            const p = e.target;
            if (e.data === PLAYING) {
              clearTimeout(watchdog);
              if (!want.current.playing) {
                p.pauseVideo();
              } else if (!want.current.muted && p.isMuted()) {
                // Playing muted, sound wanted: try it now.
                unmutedAt = Date.now();
                p.unMute();
                clearTimeout(soundCheck);
                soundCheck = setTimeout(() => {
                  if (!cancelled && unmutedAt && !want.current.muted && p.isMuted()) soundBlocked(p);
                }, UNMUTE_GRACE_MS);
              }
            } else if (e.data === PAUSED && want.current.playing) {
              if (unmutedAt && Date.now() - unmutedAt < UNMUTE_GRACE_MS) soundBlocked(p);
              else armWatchdog(p);
            } else if ((e.data === UNSTARTED || e.data === CUED) && want.current.playing) {
              armWatchdog(p);
            }
          },
        },
      });
    });

    return () => {
      cancelled = true;
      clearTimeout(watchdog);
      clearTimeout(soundCheck);
      playerRef.current?.destroy();
      playerRef.current = null;
      host.replaceChildren();
    };
  }, [videoKey]);

  // The user's own play/pause and mute taps: gestures, so sound is allowed.
  useEffect(() => {
    const p = playerRef.current;
    if (!p || typeof p.getPlayerState !== "function") return;
    if (muted) p.mute();
    else p.unMute();
    if (playing) p.playVideo();
    else p.pauseVideo();
  }, [playing, muted]);

  return (
    // "Cover" for a 16:9 video: object-cover doesn't apply to an iframe, so
    // size it to at least the full width and the full height at that ratio
    // (YouTube letterboxes anything else), then overscan: most film trailers
    // are scope (2.39:1) letterboxed in 16:9, bars 12.8% of the height each,
    // and 1.36x crops them (s >= 1 / (1 - 2 * 0.128)) along with YouTube's
    // chrome.
    <div className="pointer-events-none absolute top-1/2 left-1/2 h-[max(100dvh,56.25vw)] w-[max(100vw,177.78dvh)] -translate-x-1/2 -translate-y-1/2 scale-[1.36]">
      <div ref={hostRef} className="h-full w-full [&>iframe]:h-full [&>iframe]:w-full" />
    </div>
  );
}
