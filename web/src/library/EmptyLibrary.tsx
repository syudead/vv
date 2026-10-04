import { FolderOpen } from "lucide-react";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { EmptyState } from "../videoList/states";

/** EmptyLibrary はライブラリに動画が1本も無いときの状態で、取り込みを促す。 */
export default function EmptyLibrary({
  onScan,
  scanning,
}: {
  onScan: () => void;
  scanning: boolean;
}) {
  return (
    <EmptyState
      icon={FolderOpen}
      title={t.library.empty.title}
      description={t.library.empty.description}
      action={
        <Button size="sm" onClick={onScan} disabled={scanning}>
          {scanning ? t.list.scanning : t.list.scan}
        </Button>
      }
    />
  );
}
