import { FolderOpen } from "lucide-react";

import { t } from "../i18n";
import { useStartSetup } from "../onboarding/useStartSetup";
import { Button } from "../ui/shadcn/button";
import { EmptyState } from "../videoList/states";

/**
 * EmptyLibrary はライブラリに動画が1本も無いときの所有者の状態で、オンボードへ案内する。
 * 「Set up library」は段階を `folders` に書いてから `/welcome` を開く
 * （specs/057-home/ui-design.md「Sidebar and library」）。取り込みはそこで始める。
 */
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
