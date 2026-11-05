import { FolderSearch } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";

import type { VideoSort } from "../api/client";
import type { SmartRule } from "../api/smartFolders";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";
import SmartFolderDialog, { type SmartFolderDialogTarget } from "./SmartFolderDialog";
import { smartFolderPagePath } from "./smartFolderPaths";
import { folderSort, type LibraryFilter, ruleFromLibrary } from "./smartRule";

/**
 * The create dialog for a rule, with the toast that opens the new folder. `open` snapshots the rule
 * and replaces random sort with newest-added.
 */
export function useSaveSmartFolder() {
  const toast = useToast();
  const navigate = useNavigate();
  const [target, setTarget] = useState<SmartFolderDialogTarget | null>(null);

  const open = (rule: SmartRule, sort: VideoSort) => {
    const saved = folderSort(sort);
    setTarget({
      kind: "create",
      rule,
      sort: saved.sort,
      randomReplaced: saved.randomReplaced,
    });
  };

  const dialog = target !== null && (
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
  );
  return { open, dialog };
}

/**
 * Render only for owners with active conditions; snapshot filters on click. Creation leaves the
 * filtered list, selection and scroll intact.
 */
export default function SaveSmartFolderButton({
  filter,
  sort,
}: {
  filter: LibraryFilter;
  sort: VideoSort;
}) {
  const save = useSaveSmartFolder();
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => save.open(ruleFromLibrary(filter), sort)}
      >
        <FolderSearch aria-hidden="true" />
        {t.smartFolders.save}
      </Button>
      {save.dialog}
    </>
  );
}
