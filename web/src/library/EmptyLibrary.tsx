import { FolderOpen } from "lucide-react";

import { t } from "../i18n";
import { useStartSetup } from "../onboarding/useStartSetup";
import { Button } from "../ui/shadcn/button";
import { EmptyState } from "../videoList/states";

/** Restart setup at the folders step before opening onboarding. */
export default function EmptyLibrary() {
  const setup = useStartSetup();
  return (
    <EmptyState
      icon={FolderOpen}
      title={t.library.empty.title}
      description={t.library.empty.description}
      action={
        <Button size="sm" disabled={setup.pending} onClick={() => void setup.start()}>
          {t.library.empty.setUp}
        </Button>
      }
    />
  );
}
