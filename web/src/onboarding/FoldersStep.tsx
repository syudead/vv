import { Folder, FolderPlus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { createMediaFolder, listMediaFolders, type MediaFolder } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import FolderPicker from "../settings/FolderPicker";
import { useScanControls } from "../shell/ScanProvider";
import { ErrorState } from "../ui/patterns/error-state";
import { StepsPage } from "../ui/patterns/steps-page";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { SkipButton, StepNotice, type StepProps } from "./stepFrame";

// オンボードの「Media folders」の手順（specs/057-home/ui-design.md「Onboarding」）。足した
// フォルダの行と、メディアフォルダの選択ダイアログを開く「Add folder」。フォルダが無ければ
// 「Continue」を押せない。Continue も Skip も段階を `scan` にする。

type FolderList =
  | { status: "loading" }
  | { status: "failed"; reason: UiText }
  | { status: "ready"; folders: MediaFolder[] };

/** FolderRows は足したフォルダの行である。パスは先頭の側を省き、全体を title に置く。 */
function FolderRows({ folders }: { folders: readonly MediaFolder[] }) {
  if (folders.length === 0) return null;
  return (
    <ul
      aria-label={t.onboarding.folders.list}
      className="flex flex-col divide-y divide-border rounded-lg border border-border bg-card"
    >
      {folders.map((folder) => (
        <li key={folder.id} className="flex min-w-0 items-center gap-3 px-3 py-2 text-sm">
          <Folder aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
          <p
            title={folder.path}
            dir="rtl"
            className="min-w-0 truncate text-left font-mono"
          >
            <bdi dir="ltr">{folder.path}</bdi>
          </p>
        </li>
      ))}
    </ul>
  );
}

export default function FoldersStep({ frame, write }: StepProps) {
  const text = t.onboarding.folders;
  const scan = useScanControls();
  const [list, setList] = useState<FolderList>({ status: "loading" });
  const [picking, setPicking] = useState(false);
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<UiText | null>(null);

  // 一覧を変えるたびに増える。遅れて届いた読み込みが、足した直後の一覧と件数を巻き戻さない。
  const revision = useRef(0);
  const { setFolderCount } = scan;

  const load = useCallback(
    async (signal?: AbortSignal) => {
      revision.current += 1;
      const at = revision.current;
      setList({ status: "loading" });
      try {
        const loaded = await listMediaFolders(signal);
        if (at !== revision.current) return;
        setList({ status: "ready", folders: loaded });
        // スキャンの手順はこの件数で押せるかを決める。一覧と同じ結果から揃える。
        setFolderCount(loaded.length);
      } catch (error) {
        if (signal?.aborted || at !== revision.current) return;
        setList({ status: "failed", reason: errorText(error) });
      }
    },
    [setFolderCount],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const folders = list.status === "ready" ? list.folders : [];

  const add = async (path: string) => {
    setAdding(true);
    setAddError(null);
    try {
      const created = await createMediaFolder(path);
      revision.current += 1;
      const next = [...folders, created].sort((a, b) => a.id - b.id);
      setList({ status: "ready", folders: next });
      setFolderCount(next.length);
      setPicking(false);
    } catch (error) {
      setAddError(text.addFailed(errorText(error)));
    } finally {
      setAdding(false);
    }
  };

  return (
    <StepsPage
      {...frame}
      title={text.title}
      description={text.description}
      notice={<StepNotice message={write.error} />}
      skip={<SkipButton disabled={write.pending} onClick={() => void write.go("scan")} />}
      action={
        <Button
          size="sm"
          disabled={write.pending || folders.length === 0}
          onClick={() => void write.go("scan")}
        >
          {text.continue}
        </Button>
      }
    >
      {list.status === "loading" && (
        <div role="status" aria-label={text.loading} className="flex flex-col gap-2">
          <Skeleton className="h-9" />
        </div>
      )}
      {list.status === "failed" && (
        <ErrorState
          title={text.loadFailed(list.reason)}
          retryLabel={t.common.retry}
          onRetry={() => void load()}
        />
      )}
      {list.status === "ready" && (
        <>
          <FolderRows folders={folders} />
          <div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setAddError(null);
                setPicking(true);
              }}
            >
              <FolderPlus />
              {text.add}
            </Button>
          </div>
        </>
      )}
      {picking && (
        <FolderPicker
          folders={folders}
          submitting={adding}
          mutationError={addError}
          onClose={() => {
            if (!adding) setPicking(false);
          }}
          onSubmit={(path) => void add(path)}
        />
      )}
    </StepsPage>
  );
}
