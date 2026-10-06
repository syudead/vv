import { FolderOpen } from "lucide-react";
import { Link } from "react-router";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { EmptyState } from "../videoList/states";

/**
 * EmptyLibrary はライブラリに動画が1本も無いときの状態で、取り込みを促す。
 * 「Scan」は取り込みを始めず、開始の場所である設定の「Scan status」へ移る（issue 830）。
 */
export default function EmptyLibrary() {
  return (
    <EmptyState
      icon={FolderOpen}
      title={t.library.empty.title}
      description={t.library.empty.description}
      action={
        <Button asChild size="sm">
          <Link to="/settings#scan-status">{t.list.scan}</Link>
        </Button>
      }
    />
  );
}
