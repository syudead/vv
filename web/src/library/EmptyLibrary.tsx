import { FolderOpen } from "lucide-react";
import { Link } from "react-router";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { EmptyState } from "../videoList/states";

/** Scan navigates to settings; it does not start ingestion directly. */
export default function EmptyLibrary() {
  return (
    <EmptyState
      icon={FolderOpen}
      title={t.library.empty.title}
      description={t.library.empty.description}
      action={
        <Button asChild size="sm">
          <Link to="/settings/scan#scan-status">{t.list.scan}</Link>
        </Button>
      }
    />
  );
}
