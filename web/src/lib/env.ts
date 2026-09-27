/**
 * Centralised, validated access to public runtime config. Fail fast at
 * startup rather than surfacing a confusing "fetch failed" deep in a
 * component the first time someone forgets to set .env.local.
 */
function requireEnv(name: string, value: string | undefined): string {
  if (!value) {
    throw new Error(
      `Missing required environment variable ${name}. Copy .env.local.example to .env.local and fill it in.`,
    );
  }
  return value;
}

const configuredApiUrl = requireEnv("NEXT_PUBLIC_API_URL", process.env.NEXT_PUBLIC_API_URL).replace(/\/+$/, "");

const LOOPBACK = new Set(["localhost", "127.0.0.1", "[::1]"]);

export const env = {
  /**
   * Base URL of the API Gateway (auth, catalog, interaction, watch-party).
   *
   * A loopback URL (the local default, http://localhost:8000) follows the
   * page's own host: opened from another device on the network as
   * http://192.168.1.x:3000, the app calls http://192.168.1.x:8000, not the
   * phone's own localhost. The session cookie is SameSite=Strict, so the
   * page and the API must share a host anyway.
   */
  get apiUrl(): string {
    if (typeof window === "undefined") return configuredApiUrl;
    const url = new URL(configuredApiUrl);
    if (!LOOPBACK.has(url.hostname) || LOOPBACK.has(window.location.hostname)) return configuredApiUrl;
    url.hostname = window.location.hostname;
    return url.toString().replace(/\/+$/, "");
  },
};
