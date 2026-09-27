"use client";

import { useEffect, useRef, useState } from "react";
import { Check, Globe, Link2, Lock, LogOut, Radio, SendHorizontal, Users } from "lucide-react";
import { toast } from "sonner";

import dynamic from "next/dynamic";
const AuthDialog = dynamic(() => import("@/components/auth/auth-dialog").then((mod) => mod.AuthDialog), { ssr: false });
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWatchParty } from "@/hooks/use-watch-party";
import { displayName, idColor, initials, relativeTime } from "@/lib/format";
import { useT } from "@/i18n";
import { cn } from "@/lib/utils";

/** The page owns the single useWatchParty() call (one socket shared by this
 *  panel and the video player's play/pause controls); this component just
 *  renders that state, plus the private-party controls (lib/party.ts). */
export function WatchPartyPanel({
  wp,
  isAuthed,
  currentUserId,
  party,
  invitePath,
  onStartParty,
  onLeaveParty,
  className = "h-[calc(100vh-6rem)]",
}: {
  wp: ReturnType<typeof useWatchParty>;
  isAuthed: boolean;
  currentUserId: string | null;
  /** The private party's code, or null for the movie's open room. */
  party: string | null;
  /** The invite link's path (lib/party.ts partyHref). */
  invitePath: string | null;
  onStartParty: () => void;
  onLeaveParty: () => void;
  /** Sizing for the outer card; defaults to a near-full-viewport column. */
  className?: string;
}) {
  const [draft, setDraft] = useState("");
  const [copied, setCopied] = useState(false);
  const { t, locale } = useT();
  // Client-only component (dynamic, ssr: false), so window is there.
  const inviteUrl = invitePath ? window.location.origin + invitePath : null;

  const copyInvite = async () => {
    if (!inviteUrl) return;
    try {
      await navigator.clipboard.writeText(inviteUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
      toast.success(t("watchParty.inviteCopied"), { id: "party-invite" });
    } catch {
      toast.error(t("watchParty.copyFailed"), { id: "party-invite" });
    }
  };
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
          <h2 className="font-semibold">{t("watchParty.title")}</h2>
          <span className="flex items-center gap-1 rounded-full bg-white/5 px-2 py-0.5 text-[10px] font-semibold tracking-wide text-muted-foreground uppercase ring-1 ring-white/10">
            {party ? <Lock className="size-3" /> : <Globe className="size-3" />}
            {t(party ? "watchParty.private" : "watchParty.openRoom")}
          </span>
        </div>
        {wp.status === "open" && (
          <div className="flex items-center gap-1">
            <span className="flex items-center gap-1 px-1 text-xs text-muted-foreground" title={t("watchParty.peopleInRoom")}>
              <Users className="size-3.5" />
              {wp.participantCount}
            </span>
            {party && (
              <Button variant="ghost" size="icon" className="size-7" onClick={onLeaveParty} aria-label={t("watchParty.leave")} title={t("watchParty.leave")}>
                <LogOut className="size-3.5" />
              </Button>
            )}
          </div>
        )}
      </div>

      {/* A private party's invite: anyone with the link lands in this room. */}
      {party && inviteUrl && wp.status === "open" && (
        <div className="flex items-center gap-2 border-b border-white/10 bg-primary/[0.06] px-4 py-2">
          <Link2 className="size-3.5 shrink-0 text-primary" />
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-zinc-300" title={inviteUrl}>
            {inviteUrl.replace(/^https?:\/\//, "")}
          </span>
          <Button size="sm" variant="secondary" className="h-7 shrink-0 px-2.5 text-xs" onClick={copyInvite}>
            {copied ? <Check className="size-3.5" /> : null}
            {t(copied ? "watchParty.copied" : "watchParty.copyInvite")}
          </Button>
        </div>
      )}

      {wp.status === "open" ? (
        <>
          <div ref={feedRef} className="flex-1 space-y-3 overflow-y-auto px-4 py-3">
            {wp.feed.length === 0 && (
              <p className="py-8 text-center text-sm text-muted-foreground">
                {t(party ? "watchParty.inPrivate" : "watchParty.inOpen")}
              </p>
            )}
            {wp.feed.map((item) =>
              item.kind === "system" ? (
                <p key={item.id} className="text-center text-xs text-muted-foreground">
                  {item.note === "joined" && t("watchParty.joined", { name: displayName(item.userId, currentUserId, locale) })}
                  {item.note === "left" && t("watchParty.left", { name: displayName(item.userId, currentUserId, locale) })}
                  {item.note === "error" && item.text}
                </p>
              ) : (
                <div key={item.id} className="flex gap-2">
                  <Avatar className="size-7 shrink-0 border border-white/10">
                    <AvatarFallback
                      className="text-[10px] font-semibold text-white"
                      style={{ backgroundColor: idColor(item.userId) }}
                    >
                      {initials(item.userId, currentUserId, locale)}
                    </AvatarFallback>
                  </Avatar>
                  <div className="min-w-0">
                    <div className="flex items-baseline gap-2">
                      <span className="text-xs font-medium">{displayName(item.userId, currentUserId, locale)}</span>
                      <span className="text-[10px] text-muted-foreground">{relativeTime(item.at, locale)}</span>
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
              placeholder={t("watchParty.message")}
              maxLength={500}
              className="h-9"
            />
            <Button size="icon" className="size-9 shrink-0" onClick={send} disabled={!draft.trim()} aria-label={t("watchParty.send")}>
              <SendHorizontal className="size-4" />
            </Button>
          </div>
        </>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
          {party ? <Lock className="size-8 text-primary" strokeWidth={1.5} /> : <Radio className="size-8 text-muted-foreground" strokeWidth={1.5} />}
          <div>
            <p className="text-sm font-medium">{t(party ? "watchParty.invitedTitle" : "watchParty.togetherTitle")}</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {t(party ? "watchParty.invitedBody" : "watchParty.togetherBody")}
            </p>
          </div>
          {!isAuthed ? (
            <AuthDialog trigger={<Button size="sm">{t("watchParty.signInToJoin")}</Button>} />
          ) : party ? (
            <div className="flex flex-col items-center gap-2">
              <Button size="sm" onClick={wp.connect} disabled={wp.status === "connecting"}>
                {t(wp.status === "connecting" ? "watchParty.connecting" : "watchParty.joinParty")}
              </Button>
              <button type="button" onClick={onLeaveParty} className="text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
                {t("watchParty.openInstead")}
              </button>
            </div>
          ) : (
            <div className="flex flex-col items-center gap-2">
              <Button size="sm" onClick={onStartParty}>
                <Lock className="size-3.5" />
                {t("watchParty.startPrivate")}
              </Button>
              <Button size="sm" variant="secondary" onClick={wp.connect} disabled={wp.status === "connecting"}>
                {t(wp.status === "connecting" ? "watchParty.connecting" : "watchParty.joinOpen")}
              </Button>
            </div>
          )}
          {wp.status === "error" && (
            <p className="text-xs text-destructive">{t("watchParty.connectFailed")}</p>
          )}
          {wp.status === "closed" && <p className="text-xs text-muted-foreground">{t("watchParty.disconnected")}</p>}
        </div>
      )}
    </div>
  );
}
