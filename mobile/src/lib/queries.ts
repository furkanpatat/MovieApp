import { useMutation, useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { useAuth } from "@/store/auth";
import type { MediaType, Movie, MoviePage } from "@/types/movie";

const seg = (m: MediaType) => (m === "tv" ? "tv" : "movies");

/** Popular movies or series: GET /api/v1/{movies,tv}/popular. */
export function usePopular(mode: MediaType) {
  return useQuery({
    queryKey: [seg(mode), "popular"],
    queryFn: () => api<MoviePage>(`/api/v1/${seg(mode)}/popular?page=1`, { auth: false }),
    staleTime: 60_000,
  });
}

/** A genre's feed: GET /api/v1/discover/{movies,tv}?genre=. */
export function useGenre(mode: MediaType, genre: number) {
  return useQuery({
    queryKey: ["discover", mode, genre],
    queryFn: () => api<MoviePage>(`/api/v1/discover/${seg(mode)}?genre=${genre}&page=1`, { auth: false }),
    staleTime: 5 * 60_000,
  });
}

/** One title's details (cast, trailer, IMDb): GET /api/v1/{movies,tv}/{id}. */
export function useTitle(mode: MediaType, id: number) {
  return useQuery({
    queryKey: [seg(mode), id],
    queryFn: () => api<Movie>(`/api/v1/${seg(mode)}/${id}`, { auth: false }),
    enabled: Number.isInteger(id) && id > 0,
  });
}

/** Search: GET /api/v1/search/{movies,tv}?q=. */
export function useSearch(mode: MediaType, q: string) {
  const query = q.trim();
  return useQuery({
    queryKey: ["search", mode, query],
    queryFn: () => api<MoviePage>(`/api/v1/search/${seg(mode)}?q=${encodeURIComponent(query)}&page=1`, { auth: false }),
    enabled: query.length >= 2,
    staleTime: 60_000,
  });
}

interface LoginResponse {
  access_token: string;
  expires_in: number;
  user: { id: string; username: string };
}

/** Sign in (or sign up, then in): the token goes to the Keychain. */
export function useSignIn() {
  const signIn = useAuth((s) => s.signIn);
  return useMutation({
    mutationFn: async (input: { login: string; password: string; register?: { username: string; email: string } }) => {
      if (input.register) {
        await api("/api/v1/auth/register", { method: "POST", auth: false, body: { ...input.register, password: input.password } });
      }
      return api<LoginResponse>("/api/v1/auth/login", {
        method: "POST",
        auth: false,
        body: { login: input.register?.username ?? input.login, password: input.password },
      });
    },
    onSuccess: (r) =>
      signIn({ token: r.access_token, userId: r.user.id, username: r.user.username, expiresAt: Date.now() + r.expires_in * 1000 }),
  });
}
