import { API_URL } from "@/lib/config";
import { useAuth, type Session } from "@/store/auth";

/** Refresh this long before the access token expires. */
const EARLY_MS = 60_000;

export interface SessionResponse {
  access_token: string;
  expires_in: number;
  refresh_token?: string;
  refresh_expires_in?: number;
  user: { id: string; username: string };
}

export function toSession(r: SessionResponse, previous?: Session): Session {
  const now = Date.now();
  return {
    token: r.access_token,
    expiresAt: now + r.expires_in * 1000,
    refreshToken: r.refresh_token ?? previous?.refreshToken,
    refreshExpiresAt: r.refresh_expires_in ? now + r.refresh_expires_in * 1000 : previous?.refreshExpiresAt,
    userId: r.user.id,
    username: r.user.username,
  };
}

// One refresh at a time: the server rotates refresh tokens and treats a
// spent one coming back as theft (revoking the session), so two parallel
// refreshes with the same token would sign the user out.
let inflight: Promise<string | null> | null = null;

/**
 * A usable access token: the current one, or a new one from the refresh
 * token when it's (nearly) expired or `force` (the API said 401). null means
 * signed out. A network failure keeps the session (the next call retries).
 */
export function freshToken(force = false): Promise<string | null> {
  const s = useAuth.getState().session;
  if (!s) return Promise.resolve(null);
  if (!force && s.expiresAt - Date.now() > EARLY_MS) return Promise.resolve(s.token);
  if (!s.refreshToken) {
    if (force || s.expiresAt <= Date.now()) void useAuth.getState().signOut();
    return Promise.resolve(force || s.expiresAt <= Date.now() ? null : s.token);
  }
  inflight ??= refresh(s).finally(() => {
    inflight = null;
  });
  return inflight;
}

async function refresh(s: Session): Promise<string | null> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ refresh_token: s.refreshToken }),
    });
  } catch {
    return s.expiresAt > Date.now() ? s.token : null; // offline: try again later
  }
  if (res.status === 401) {
    await useAuth.getState().signOut(); // spent, revoked or expired: sign in again
    return null;
  }
  if (!res.ok) return s.expiresAt > Date.now() ? s.token : null;
  const next = toSession((await res.json()) as SessionResponse, s);
  await useAuth.getState().signIn(next);
  return next.token;
}

/** Sign out here and end the session on the server (revoke its refresh token). */
export async function logout(): Promise<void> {
  const refreshToken = useAuth.getState().session?.refreshToken;
  await useAuth.getState().signOut();
  if (!refreshToken) return;
  try {
    await fetch(`${API_URL}/api/v1/auth/logout`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
  } catch {
    // Offline: the token still expires on its own.
  }
}

/** A failed account deletion; wrongPassword: the password was refused. */
export class AccountDeletionError extends Error {
  constructor(public wrongPassword: boolean) {
    super(wrongPassword ? "wrong password" : "account deletion failed");
  }
}

/**
 * Permanently deletes the user's account and data (the gateway checks the
 * password, then removes it across the services), then the local session.
 */
export async function deleteAccount(password: string): Promise<void> {
  const token = await freshToken();
  if (!token) throw new AccountDeletionError(false);
  let res: Response;
  try {
    res = await fetch(`${API_URL}/api/v1/account/delete`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: `Bearer ${token}` },
      body: JSON.stringify({ password }),
    });
  } catch {
    throw new AccountDeletionError(false);
  }
  if (!res.ok) throw new AccountDeletionError(res.status === 403); // 403: the password was wrong
  await useAuth.getState().signOut();
}
