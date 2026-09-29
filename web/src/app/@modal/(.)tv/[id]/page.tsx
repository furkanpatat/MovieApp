"use client";

import { useParams } from "next/navigation";

import { useT } from "@/i18n";

import { TVDetail } from "@/components/title/tv-detail";
import { TitleModal } from "@/components/title/title-modal";

/** A client-side navigation to /tv/[id]: the series opens in a modal (see
 *  (.)movies/[id]). */
export default function TVModal() {
  const { id } = useParams<{ id: string }>();
  const { t } = useT();
  return (
    <TitleModal label={t("detail.seriesDetails")}>
      <TVDetail showId={Number(id)} variant="modal" />
    </TitleModal>
  );
}
