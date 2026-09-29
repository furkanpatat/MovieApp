"use client";

import { useT } from "@/i18n";

import { PersonDetail } from "@/components/person/person-detail";
import { TitleModal } from "@/components/title/title-modal";

/** A client-side navigation to /person/[id] (a cast member in a title's
 *  details, on Home, Discover...): the person opens in the modal over the
 *  current page, with the same close button, Escape and outside click as a
 *  title. Closing goes back, e.g. to the movie the cast member came from. */
export default function PersonModal() {
  const { t } = useT();
  return (
    <TitleModal label={t("person.details")}>
      <PersonDetail />
    </TitleModal>
  );
}
