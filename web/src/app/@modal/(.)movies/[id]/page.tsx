"use client";

import { useParams } from "next/navigation";

import { useT } from "@/i18n";

import { MovieDetail } from "@/components/title/movie-detail";
import { TitleModal } from "@/components/title/title-modal";

/** A client-side navigation to /movies/[id] (a card on Home, Discover,
 *  search...): the movie opens in a modal over the current page. `(.)` as
 *  @modal is a slot, not a segment: this intercepts the top-level /movies. */
export default function MovieModal() {
  const { id } = useParams<{ id: string }>();
  const { t } = useT();
  return (
    <TitleModal label={t("detail.movieDetails")}>
      <MovieDetail movieId={Number(id)} variant="modal" />
    </TitleModal>
  );
}
