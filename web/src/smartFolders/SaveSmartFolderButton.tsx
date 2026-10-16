import { FolderSearch } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";

import type { VideoSort } from "../api/client";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";
import SmartFolderDialog, { type SmartFolderDialogTarget } from "./SmartFolderDialog";
import { smartFolderPagePath } from "./smartFolderPaths";
import { folderSort, type LibraryFilter, ruleFromLibrary } from "./smartRule";

/**
 * SaveSmartFolderButton はライブラリの見出しの「Save as smart folder」と、それが開く窓である
 * （specs/052-smart-folders/ui-design.md「Save as smart folder」）。呼び手は所有者で条件が
 * 効いている間だけ描く（条件の無いときは出さない、要件 10）。
 *
 * 押した時点の絞り込みを条件に、並べ順を今の並べ順に（ランダムなら追加日の新しい順に）して
 * 窓を開く。作れたら `Created "{name}"` のトーストに、新しいフォルダのページを開く `Open` を
 * 添える。絞り込んだ一覧と選択とスクロールはそのまま（絞り込んだライブラリにフォルダの
 * カードは並ばない、R-7）。
 */
export default function SaveSmartFolderButton({
  filter,
  sort,
}: {
  filter: LibraryFilter;
  sort: VideoSort;
}) {
  const toast = useToast();
  const navigate = useNavigate();
  const [target, setTarget] = useState<SmartFolderDialogTarget | null>(null);

  const open = () => {
    const saved = folderSort(sort);
    setTarget({
      kind: "create",
      rule: ruleFromLibrary(filter),
      sort: saved.sort,
      randomReplaced: saved.randomReplaced,
    });
  };

  return (
    <>
      <Button variant="outline" size="sm" onClick={open}>
        <FolderSearch aria-hidden="true" />
        {t.smartFolders.save}
      </Button>
      {target !== null && (
        <SmartFolderDialog
          target={target}
          onClose={() => setTarget(null)}
          onDone={(folder) => {
            setTarget(null);
            toast(t.smartFolders.created(folder.name), {
              action: {
                label: t.smartFolders.open,
                run: () => {
                  void navigate(smartFolderPagePath(folder.id));
                  return Promise.resolve();
                },
              },
            });
          }}
        />
      )}
    </>
  );
}
