import { useAuth } from "@/store/auth";

/**
 * The KinoCut API (the Go gateway). A native app has no cookies to rely on,
 * so it sends the session as `Authorization: Bearer` (the gateway accepts
 * both); cookie-only protections like the Origin check don't apply to it.
 * EXPO_PUBLIC_API_URL points it elsewhere (e.g. your Mac's LAN address for
 * the local stack); the default is the live server.
 */
export const API_URL = (process.env.EXPO_PUBLIC_API_URL ?? "https://api.kinora.duckdns.org").replace(/\/+$/, "");

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
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const token = useAuth.getState().token;
  if (auth && token) headers.Authorization = `Bearer ${token}`;

  const res = await fetch(API_URL + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const text = await res.text();
  const data = text ? safeJSON(text) : undefined;
  if (!res.ok) {
    // An expired or revoked session: forget it, the user signs in again.
    if (res.status === 401 && auth && token) void useAuth.getState().signOut();
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
