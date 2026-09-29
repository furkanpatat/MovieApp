"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useT } from "@/i18n";
import { ApiError, apiFetch } from "@/lib/api-client";
import { useAuthStore } from "@/store/auth-store";

/**
 * Delete account (the stores and GDPR ask for it in the app): confirmed with
 * the password. The gateway deletes the account across the services and
 * clears the session cookie; then the local session goes too.
 */
export function DeleteAccountDialog() {
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const router = useRouter();
  const queryClient = useQueryClient();
  const clearSession = useAuthStore((s) => s.clearSession);
  const { t } = useT();

  const del = useMutation({
    mutationFn: () => apiFetch<void>("/api/v1/account/delete", { method: "POST", body: { password } }),
    onSuccess: () => {
      clearSession();
      queryClient.clear();
      setOpen(false);
      router.replace("/");
      toast.success(t("profile.deleted"));
    },
  });

  const error =
    del.error instanceof ApiError && del.error.status === 403
      ? t("profile.deleteWrongPassword")
      : del.error
        ? t("profile.deleteFailed")
        : null;

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (!o) {
          setPassword("");
          del.reset();
        }
      }}
    >
      <DialogTrigger asChild>
        <Button variant="ghost" className="w-full text-red-400 hover:bg-red-500/10 hover:text-red-300 sm:w-auto">
          <Trash2 className="size-4" />
          {t("profile.deleteAccount")}
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-red-400">
            <AlertTriangle className="size-5" />
            {t("profile.deleteAccount")}
          </DialogTitle>
          <DialogDescription>{t("profile.deleteWarning")}</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (password && !del.isPending) del.mutate();
          }}
        >
          <label htmlFor="delete-password" className="text-sm font-medium">
            {t("profile.deleteConfirm")}
          </label>
          <Input
            id="delete-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-invalid={!!error}
          />
          {error && <p className="text-sm text-red-400">{error}</p>}
          <DialogFooter className="pt-4">
            <Button type="button" variant="secondary" onClick={() => setOpen(false)} disabled={del.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="destructive" disabled={!password || del.isPending}>
              {del.isPending ? t("profile.deleting") : t("profile.deleteForever")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
