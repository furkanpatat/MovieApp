"use client";

import { useParams } from "next/navigation";

import { MovieDetail } from "@/components/title/movie-detail";

/** /movies/[id] opened directly (a shared link, a reload): the full page.
 *  Client-side navigations from the app open @modal/(.)movies/[id] instead. */
export default function MovieDetailPage() {
  const { id } = useParams<{ id: string }>();
  return <MovieDetail movieId={Number(id)} />;
}
