import * as SecureStore from "expo-secure-store";
import { create } from "zustand";

const KEY = "kinocut.session";

export interface Session {
  /** The access token: a short-lived JWT (about an hour). */
  token: string;
  /** Exchanged for the next access token (lib/session.ts); ~30 days. */
  refreshToken?: string;
  userId: string;
  username: string;
  /** Unix ms. */
  expiresAt: number;
  refreshExpiresAt?: number;
}

interface AuthState {
  session: Session | null;
  token: string | null;
  userId: string | null;
  username: string | null;
  /** The saved session has been read (don't show "signed out" before). */
  ready: boolean;
  restore: () => Promise<void>;
  signIn: (s: Session) => Promise<void>;
  /** Forget the session on this device (lib/session.ts logout also revokes it). */
  signOut: () => Promise<void>;
}

const usable = (s: Session) => s.expiresAt > Date.now() || (!!s.refreshToken && (s.refreshExpiresAt ?? 0) > Date.now());

/**
 * The session, kept in the Keychain (iOS) / Keystore (Android) via
 * expo-secure-store, never in plain storage. It survives the access token's
 * expiry while its refresh token is good.
 */
export const useAuth = create<AuthState>((set) => ({
  session: null,
  token: null,
  userId: null,
  username: null,
  ready: false,
  restore: async () => {
    try {
      const raw = await SecureStore.getItemAsync(KEY);
      const s = raw ? (JSON.parse(raw) as Session) : null;
      if (s && usable(s)) set({ session: s, token: s.token, userId: s.userId, username: s.username });
      else if (s) await SecureStore.deleteItemAsync(KEY);
    } catch {
      // Unreadable: start signed out.
    }
    set({ ready: true });
  },
  signIn: async (s) => {
    await SecureStore.setItemAsync(KEY, JSON.stringify(s));
    set({ session: s, token: s.token, userId: s.userId, username: s.username });
  },
  signOut: async () => {
    await SecureStore.deleteItemAsync(KEY);
    set({ session: null, token: null, userId: null, username: null });
  },
}));
