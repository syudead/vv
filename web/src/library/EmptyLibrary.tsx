import { FolderMinus, FolderOpen } from "lucide-react";
import { Link } from "react-router";

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

/** Every media folder is switched off; the switches are on the Library settings page. */
export function AllFoldersOff() {
  const text = t.library.allFoldersOff;
  return (
    <EmptyState
      icon={FolderMinus}
      title={text.title}
      description={text.description}
      action={
        <Button asChild size="sm">
          <Link to="/settings/library">{text.open}</Link>
        </Button>
      }
    />
  );
}
