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

export const env = {
  /** Base URL of the API Gateway (auth, catalog, interaction, watch-party). */
  apiUrl: requireEnv(
    "NEXT_PUBLIC_API_URL",
    process.env.NEXT_PUBLIC_API_URL,
  ).replace(/\/+$/, ""),
};
