import { PersonDetail } from "@/components/person/person-detail";

/** An actor or crew member as a full page: a reload or a shared link. A
 *  click inside the app opens the same details in the modal instead
 *  (@modal/(.)person/[id]). */
export default function PersonPage() {
  return <PersonDetail />;
}
