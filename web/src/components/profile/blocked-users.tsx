"use client";

import { Loader2 } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { useBlockedUsers, useBlockUser } from "@/hooks/queries";
import { useT } from "@/i18n";
import { displayName, idColor, initials } from "@/lib/format";
import { useAuthStore } from "@/store/auth-store";

/** The users you've blocked (their comments are hidden), with a way back.
 *  Shown only when there is someone. */
export function BlockedUsers({ className }: { className?: string }) {
  const { t, locale } = useT();
  const me = useAuthStore((s) => s.userId);
  const blocked = useBlockedUsers().data ?? [];
  const unblock = useBlockUser();
  if (blocked.length === 0) return null;

  return (
    <section className={className}>
      <h2 className="text-lg font-bold tracking-tight">{t("profile.blockedTitle")}</h2>
      <p className="mt-1 text-xs text-muted-foreground">{t("profile.blockedNote")}</p>
      <ul className="mt-4 divide-y divide-white/5">
        {blocked.map((id) => (
          <li key={id} className="flex items-center gap-3 py-3">
            <Avatar className="size-8 shrink-0 border border-white/10">
              <AvatarFallback className="text-xs font-semibold text-white" style={{ backgroundColor: idColor(id) }}>
                {initials(id, me, locale)}
              </AvatarFallback>
            </Avatar>
            <span className="min-w-0 flex-1 truncate text-sm">{displayName(id, me, locale)}</span>
            <Button
              size="sm"
              variant="secondary"
              disabled={unblock.isPending && unblock.variables?.target === id}
              onClick={() => unblock.mutate({ target: id, block: false })}
            >
              {unblock.isPending && unblock.variables?.target === id && <Loader2 className="size-4 animate-spin" />}
              {t("profile.unblock")}
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}
