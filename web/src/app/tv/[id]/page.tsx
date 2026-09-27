"use client";

import { useParams } from "next/navigation";

import { TVDetail } from "@/components/title/tv-detail";

/** /tv/[id] opened directly: the full page (see /movies/[id]). */
export default function TVDetailPage() {
  const { id } = useParams<{ id: string }>();
  return <TVDetail showId={Number(id)} />;
}
