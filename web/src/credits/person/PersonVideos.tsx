import { t } from "../../i18n";
import { CreditShelves, type ShelfSubject } from "../CreditShelves";

const roles = (["performer", "director", "photographer", "streamer"] as const).map(
  (role) => ({ role, title: t.credits.personPage.roles[role] }),
);

/** One shelf per role the person has videos in; a role whose first read fails is left out. */
export function PersonShelves({ personId }: { personId: number }) {
  const subject: ShelfSubject = { kind: "person", id: personId, roles };
  return <CreditShelves subject={subject} />;
}
