/**
 * The KinoCut API (the Go gateway). EXPO_PUBLIC_API_URL points the app
 * elsewhere (e.g. your Mac's LAN address for the local stack); the default
 * is the live server.
 */
export const API_URL = (process.env.EXPO_PUBLIC_API_URL ?? "https://api.kinora.duckdns.org").replace(/\/+$/, "");
