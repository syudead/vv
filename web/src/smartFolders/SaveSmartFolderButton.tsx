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
 * Render only for owners with active conditions; snapshot filters on click and replace random sort
 * with newest-added. Creation leaves the filtered list, selection and scroll intact.
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
