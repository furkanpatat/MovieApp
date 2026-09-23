"use client";

import { useEffect, useRef, useState } from "react";
import { Radio, SendHorizontal, Users } from "lucide-react";

import dynamic from "next/dynamic";
const AuthDialog = dynamic(() => import("@/components/auth/auth-dialog").then((mod) => mod.AuthDialog), { ssr: false });
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWatchParty } from "@/hooks/use-watch-party";
import { displayName, idColor, initials, relativeTime } from "@/lib/format";
import { cn } from "@/lib/utils";

/** The page owns the single useWatchParty() call (one socket shared by this
 *  panel and the video player's play/pause controls); this component just
 *  renders that state. */
export function WatchPartyPanel({
  wp,
  isAuthed,
  currentUserId,
  className = "h-[calc(100vh-6rem)]",
}: {
  wp: ReturnType<typeof useWatchParty>;
  isAuthed: boolean;
  currentUserId: string | null;
  /** Sizing for the outer card; defaults to a near-full-viewport column. */
  className?: string;
}) {
  const [draft, setDraft] = useState("");
  const feedRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = feedRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [wp.feed.length]);

  const send = () => {
    const text = draft.trim();
    if (!text) return;
    wp.sendChat(text);
    setDraft("");
  };

  return (
    <div className={cn("flex flex-col overflow-hidden rounded-xl border border-white/10 bg-card", className)}>
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
        <div className="flex items-center gap-2">
          <Radio className={`size-4 ${wp.status === "open" ? "text-primary" : "text-muted-foreground"}`} />
          <h2 className="font-semibold">Watch Party</h2>
        </div>
        {wp.status === "open" && (
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <Users className="size-3.5" />
            {wp.participantCount}
          </span>
        )}
      </div>

      {wp.status === "open" ? (
        <>
          <div ref={feedRef} className="flex-1 space-y-3 overflow-y-auto px-4 py-3">
            {wp.feed.length === 0 && (
              <p className="py-8 text-center text-sm text-muted-foreground">
                You&apos;re in. Say hello 👋
              </p>
            )}
            {wp.feed.map((item) =>
              item.kind === "system" ? (
                <p key={item.id} className="text-center text-xs text-muted-foreground">
                  {item.note === "joined" && `${displayName(item.userId, currentUserId)} joined the party`}
                  {item.note === "left" && `${displayName(item.userId, currentUserId)} left`}
                  {item.note === "error" && item.text}
                </p>
              ) : (
                <div key={item.id} className="flex gap-2">
                  <Avatar className="size-7 shrink-0 border border-white/10">
                    <AvatarFallback
                      className="text-[10px] font-semibold text-white"
                      style={{ backgroundColor: idColor(item.userId) }}
                    >
                      {initials(item.userId, currentUserId)}
                    </AvatarFallback>
                  </Avatar>
                  <div className="min-w-0">
                    <div className="flex items-baseline gap-2">
                      <span className="text-xs font-medium">{displayName(item.userId, currentUserId)}</span>
                      <span className="text-[10px] text-muted-foreground">{relativeTime(item.at)}</span>
                    </div>
                    <p className="text-sm break-words">{item.text}</p>
                  </div>
                </div>
              ),
            )}
          </div>
          <div className="flex items-center gap-2 border-t border-white/10 p-3">
            <Input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") send();
              }}
              placeholder="Send a message…"
              maxLength={500}
              className="h-9"
            />
            <Button size="icon" className="size-9 shrink-0" onClick={send} disabled={!draft.trim()} aria-label="Send">
              <SendHorizontal className="size-4" />
            </Button>
          </div>
        </>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
          <Radio className="size-8 text-muted-foreground" strokeWidth={1.5} />
          <div>
            <p className="text-sm font-medium">Watch this together</p>
            <p className="mt-1 text-xs text-muted-foreground">
              Chat and sync play/pause in real time with everyone else here.
            </p>
          </div>
          {!isAuthed ? (
            <AuthDialog trigger={<Button size="sm">Sign in to join</Button>} />
          ) : (
            <Button size="sm" onClick={wp.connect} disabled={wp.status === "connecting"}>
              {wp.status === "connecting" ? "Connecting…" : "Join Watch Party"}
            </Button>
          )}
          {wp.status === "error" && (
            <p className="text-xs text-destructive">Couldn&apos;t connect. Please try again.</p>
          )}
          {wp.status === "closed" && <p className="text-xs text-muted-foreground">Disconnected.</p>}
        </div>
      )}
    </div>
  );
}
