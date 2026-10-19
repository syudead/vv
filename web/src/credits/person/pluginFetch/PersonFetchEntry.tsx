import { UserRoundSearch } from "lucide-react";
import { useState } from "react";

import type { Person } from "../../../api/credits";
import { t } from "../../../i18n";
import { useApprovedPlugins } from "../../../player/workInfoFetch/FetchEntry";
import { approvedPersonPlugins } from "../../../player/workInfoFetch/fetchRules";
import { Button } from "../../../ui/shadcn/button";
import PersonFetchDialog from "./PersonFetchDialog";

/** Fetch info…, shown once an approved person plugin exists; an open dialog stays when the list changes. */
export default function PersonFetchEntry({
  person,
  onApplied,
  onGone,
}: {
  person: Person;
  onApplied: (person: Person) => void;
  onGone: () => void;
}) {
  const { plugins, reload } = useApprovedPlugins(approvedPersonPlugins);
  const [open, setOpen] = useState(false);
  return (
    <>
      {plugins.length > 0 && (
        <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
          <UserRoundSearch aria-hidden="true" />
          {t.credits.personPage.pluginFetch.entry}
        </Button>
      )}
      {open && (
        <PersonFetchDialog
          person={person}
          plugins={plugins}
          onReload={reload}
          onApplied={onApplied}
          onGone={onGone}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
