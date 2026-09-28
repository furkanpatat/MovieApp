import { getRandomBytes } from "expo-crypto";

import type { Dictionary } from "@/i18n/en";

/**
 * Watch parties, named exactly as on the web (web/src/lib/party.ts), so the
 * phone and the site meet in the same rooms: every movie has an open room
 * (`movie-{id}`) and private ones reached by invite (`movie-{id}-{code}`,
 * link /movies/{id}?party={code}).
 */
const ALPHABET = "abcdefghjkmnpqrstuvwxyz23456789"; // no look-alikes (0/o, 1/l/i)
const CODE = /^[a-z0-9]{6,16}$/;
export const SITE = "https://kinora.duckdns.org";

/** A fresh private-party code: 10 characters, ~49 bits. */
export function newPartyCode(): string {
  return Array.from(getRandomBytes(10), (b) => ALPHABET[b % ALPHABET.length]).join("");
}

export function parsePartyCode(value: string | string[] | undefined): string | null {
  const v = Array.isArray(value) ? value[0] : value;
  return v && CODE.test(v) ? v : null;
}

export function partyRoom(movieId: number, code: string | null): string {
  return code ? `movie-${movieId}-${code}` : `movie-${movieId}`;
}

/** The invite link: opens the party on the web (or here, pasted). */
export function inviteUrl(movieId: number, code: string): string {
  return `${SITE}/movies/${movieId}?party=${code}`;
}

/** "You", or a short, stable label for someone else (as on the web). */
export function displayName(userId: string, me: string | null, t: Dictionary): string {
  return me && userId === me ? t.common.you : t.common.user(userId.slice(0, 8));
}
