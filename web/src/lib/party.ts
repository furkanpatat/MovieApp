/**
 * Watch parties. Every movie has an open room anyone watching can join
 * (`movie-{id}`), and any number of private ones, each reached through its
 * invite link: /movies/{id}?party={code} (room `movie-{id}-{code}`). The
 * Watch-Party service creates a room when its first member joins and drops
 * it when the last one leaves; a code is just an unguessable room name.
 */

const ALPHABET = "abcdefghjkmnpqrstuvwxyz23456789"; // no look-alikes (0/o, 1/l/i)
const CODE = /^[a-z0-9]{6,16}$/;

/** A fresh private-party code: 10 characters, ~49 bits. */
export function newPartyCode(): string {
  const bytes = new Uint8Array(10);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => ALPHABET[b % ALPHABET.length]).join("");
}

/** The code from a ?party= value, or null when absent or malformed. */
export function parsePartyCode(value: string | null): string | null {
  return value && CODE.test(value) ? value : null;
}

export function partyRoom(movieId: number, code: string | null): string {
  return code ? `movie-${movieId}-${code}` : `movie-${movieId}`;
}

export function partyHref(movieId: number, code: string): string {
  return `/movies/${movieId}?party=${code}`;
}
