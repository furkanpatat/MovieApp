import { useMutation } from "@tanstack/react-query";

import { apiFetch } from "@/lib/api-client";
import { useAuthStore } from "@/store/auth-store";
import type { LoginResponse, RegisterResponse } from "@/types/auth";

/** Signs in. The Auth service sets the HttpOnly session cookie on this
 *  response; the body's access_token is for non-browser clients and is
 *  deliberately not kept, so no script can ever read the credential. */
export function useLogin() {
  const setSession = useAuthStore((s) => s.setSession);
  return useMutation({
    mutationFn: (input: { login: string; password: string }) =>
      apiFetch<LoginResponse>("/api/v1/auth/login", { method: "POST", body: input, auth: false }),
    onSuccess: (data) => {
      setSession({
        userId: data.user.id,
        username: data.user.username,
        email: data.user.email ?? null,
        expiresAt: Date.now() + data.expires_in * 1000,
      });
    },
  });
}

/** Signs out: the server must clear the cookie (JS can't touch an HttpOnly
 *  one). The local session is dropped even if that call fails, so the UI
 *  never stays "signed in" against the user's wishes. */
export function useLogout() {
  const clearSession = useAuthStore((s) => s.clearSession);
  return useMutation({
    mutationFn: () => apiFetch<void>("/api/v1/auth/logout", { method: "POST", auth: false }),
    onSettled: () => clearSession(),
  });
}

export function useRegister() {
  return useMutation({
    mutationFn: (input: { username: string; email: string; password: string }) =>
      apiFetch<RegisterResponse>("/api/v1/auth/register", { method: "POST", body: input, auth: false }),
  });
}
