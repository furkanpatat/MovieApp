import type { Movie } from "@/types/movie";

export type ChatRole = "user" | "assistant";

/** POST /api/v1/chat request: the recent conversation, oldest first, and
 *  where the user is (the server looks up what's on that page itself). */
export interface ChatRequest {
  messages: { role: ChatRole; content: string }[];
  context?: { path: string };
  /** The UI language: the concierge replies in it. */
  locale?: "en" | "tr";
}

/** POST /api/v1/chat response. `message` is Markdown; `movies` are the
 *  `movie_ids` resolved to card data by the catalog. */
export interface ChatResponse {
  message: string;
  movie_ids: number[];
  movies: Movie[];
  /** Canned replies: no LLM is configured on the server. */
  demo?: boolean;
}

/** One message as the chat panel renders it. Rich content hangs off the
 *  text as optional blocks, so new kinds (trailers, lists...) slot in
 *  beside `movies` without touching the plain-text path. */
export interface ChatTurn {
  id: string;
  role: ChatRole;
  content: string;
  movies?: Movie[];
  /** A failed request; shown with a retry, never sent back to the model. */
  failed?: boolean;
}
