import { toast } from "sonner";

import { currentLocale, translate } from "@/i18n";

import { env } from "@/lib/env";
import { useAuthStore } from "@/store/auth-store";

/** The shape every Gateway/service error response takes. */
interface ApiErrorBody {
  error?: string;
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly body?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

interface RequestOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
  /** The call needs the signed-in session: a 401 then means it has ended
   *  (expired, or signed out elsewhere), so the local session is cleared.
   *  Default true; public endpoints and the sign-in calls pass false. */
  auth?: boolean;
}

/**
 * Thin fetch wrapper for the API Gateway. Handles JSON in/out, sends the
 * HttpOnly session cookie, and normalises every failure into an ApiError so
 * callers can branch on `.status` (401 -> sign in again, 429 -> retry later,
 * 0 -> unreachable) instead of re-parsing the body everywhere.
 */
export async function apiFetch<T>(
  path: string,
  { body, auth = true, headers, ...init }: RequestOptions = {},
): Promise<T> {
  const finalHeaders = new Headers(headers);
  finalHeaders.set("Accept", "application/json");
  if (body !== undefined) finalHeaders.set("Content-Type", "application/json");

  let res: Response;
  try {
    res = await fetch(`${env.apiUrl}${path}`, {
      ...init,
      headers: finalHeaders,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      // The API is another origin (its own port), so cookies are only sent,
      // and Set-Cookie only honoured, with credentials: "include".
      credentials: "include",
    });
  } catch {
    throw new ApiError(translate(currentLocale(), "common.offline"), 0);
  }

  // 202 Accepted (our CQRS write path) and 204 have no body to parse.
  const text = await res.text();
  const data = text ? (JSON.parse(text) as unknown) : undefined;

  if (!res.ok) {
    if (res.status === 401 && auth && useAuthStore.getState().username !== null) {
      useAuthStore.getState().clearSession();
      toast(translate(currentLocale(), "common.sessionEnded"), { id: "session-ended" });
    }
    const message =
      (data as ApiErrorBody | undefined)?.error ?? `Request failed (${res.status})`;
    throw new ApiError(message, res.status, data);
  }
  return data as T;
}
