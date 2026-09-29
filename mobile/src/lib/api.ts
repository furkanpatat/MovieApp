import { API_URL } from "@/lib/config";
import { freshToken } from "@/lib/session";
import { useAuth } from "@/store/auth";

/**
 * Calls the KinoCut API. A native app has no cookies to rely on, so it
 * sends the session as `Authorization: Bearer` (the gateway accepts both);
 * cookie-only protections like the Origin check don't apply to it. The
 * token is refreshed as needed (lib/session.ts); a 401 gets one retry with
 * a freshly refreshed token before the user is signed out.
 */
export { API_URL };

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

type Options = { method?: string; body?: unknown; auth?: boolean };

export async function api<T>(path: string, { method = "GET", body, auth = true }: Options = {}): Promise<T> {
  const send = (token: string | null) => {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (token) headers.Authorization = `Bearer ${token}`;
    return fetch(API_URL + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  };

  const token = auth ? await freshToken() : null;
  let res = await send(token);
  if (res.status === 401 && token) {
    // Revoked or expired early: one refresh, one retry.
    const retry = await freshToken(true);
    res = retry ? await send(retry) : res;
    if (res.status === 401) await useAuth.getState().signOut();
  }
  const text = await res.text();
  const data = text ? safeJSON(text) : undefined;
  if (!res.ok) {
    const message = (data as { error?: string } | undefined)?.error ?? `HTTP ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return data as T;
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}
