import { FolderOpen } from "lucide-react";

import Button from "../ui/Button";
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
      title="動画がまだありません"
      description="メディアフォルダに動画を置いて取り込むと、ここに並びます。"
      action={
        <Button variant="primary" onClick={onScan} disabled={scanning}>
          {scanning ? "取り込み中…" : "取り込む"}
        </Button>
      }
    />
  );
}
