import { Folder, FolderPlus, LoaderCircle, Pencil, Trash2 } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  createMediaFolder,
  deleteMediaFolder,
  errorMessage,
  listMediaFolders,
  RequestFailed,
  updateMediaFolder,
  type MediaFolder,
} from "../api/client";
import { useScan } from "../shell/ScanProvider";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import Skeleton from "../ui/Skeleton";
import { useToast } from "../ui/Toast";
import FolderPicker, { ModalFrame } from "./FolderPicker";

type Pending = { id: number | "new"; kind: "add" | "change" | "delete" } | null;

function DeleteDialog({
  folder,
  pending,
  error,
  onClose,
  onDelete,
}: {
  folder: MediaFolder;
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onDelete: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  return (
    <ModalFrame title="フォルダの削除を確認" onClose={onClose} initialFocus={cancel}>
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4 sm:p-5">
        <code className="break-words text-sm text-fg">{folder.path}</code>
        <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
          このフォルダだけにある動画は一覧から外れます。別の登録フォルダにもある動画は残ります。再生位置と視聴済み状態は残ります。取り込みは自動では始まりません。
        </p>
        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 justify-end gap-2 border-t border-border p-4">
        <Button ref={cancel} onClick={onClose} disabled={pending}>
          キャンセル
        </Button>
        <Button variant="danger" onClick={onDelete} disabled={pending}>
          {pending && <LoaderCircle className="animate-spin" />}
          削除する
        </Button>
      </div>
    </ModalFrame>
  );
}

export default function SettingsPage() {
  const scan = useScan();
  const toast = useToast();
  const [folders, setFolders] = useState<MediaFolder[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [picker, setPicker] = useState<MediaFolder | "add" | null>(null);
  const [deleting, setDeleting] = useState<MediaFolder | null>(null);
  const [pending, setPending] = useState<Pending>(null);
  const [operationError, setOperationError] = useState<string | null>(null);
  const [rowError, setRowError] = useState<{ id: number; message: string } | null>(null);
  const rowRefs = useRef(new Map<number, HTMLDivElement>());
  const addButton = useRef<HTMLButtonElement>(null);

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true);
      setLoadError(null);
      try {
        const result = await listMediaFolders(signal);
        const sorted = [...result].sort((a, b) => a.id - b.id);
        setFolders(sorted);
        scan.setFolderCount(result.length);
        return sorted;
      } catch (failure) {
        if (signal?.aborted) return;
        setLoadError(errorMessage(failure));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [scan.setFolderCount],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const focusFolderAction = (folder?: MediaFolder) => {
    setTimeout(() => {
      if (folder === undefined) addButton.current?.focus();
      else
        rowRefs.current
          .get(folder.id)
          ?.querySelector<HTMLButtonElement>("button")
          ?.focus();
    }, 0);
  };

  const handleFailure = async (failure: unknown, folder?: MediaFolder) => {
    if (
      failure instanceof RequestFailed &&
      (failure.code === "conflict" || failure.code === "media_folder_not_found") &&
      folder
    ) {
      setRowError({
        id: folder.id,
        message: "別の画面で変更または削除されました。内容を確認してやり直してください",
      });
      setPicker(null);
      setDeleting(null);
      const targetIndex = folders.findIndex((candidate) => candidate.id === folder.id);
      const refreshed = await load();
      if (refreshed !== undefined) {
        const nextFolder =
          refreshed[Math.min(Math.max(targetIndex, 0), refreshed.length - 1)];
        focusFolderAction(nextFolder);
      }
      return;
    }
    if (failure instanceof RequestFailed && failure.code === "scan_in_progress") {
      setOperationError("取り込み中はメディアフォルダを変更できません");
      scan.refresh();
      return;
    }
    setOperationError(errorMessage(failure));
  };

  const submitFolder = async (path: string) => {
    const replacing = picker === "add" ? undefined : (picker ?? undefined);
    setOperationError(null);
    setPending({ id: replacing?.id ?? "new", kind: replacing ? "change" : "add" });
    try {
      if (replacing === undefined) {
        const created = await createMediaFolder(path);
        setFolders((current) => [...current, created].sort((a, b) => a.id - b.id));
        scan.setFolderCount(folders.length + 1);
        toast("追加しました。反映するには取り込みを実行してください");
      } else {
        const updated = await updateMediaFolder(replacing.id, path, replacing.version);
        setFolders((current) =>
          current.map((folder) => (folder.id === updated.id ? updated : folder)),
        );
        setRowError((current) => (current?.id === updated.id ? null : current));
        toast("変更しました。反映するには取り込みを実行してください");
      }
      setPicker(null);
    } catch (failure) {
      await handleFailure(failure, replacing);
    } finally {
      setPending(null);
    }
  };

  const removeFolder = async () => {
    if (deleting === null) return;
    const target = deleting;
    setOperationError(null);
    setPending({ id: target.id, kind: "delete" });
    try {
      await deleteMediaFolder(target.id, target.version);
      const remaining = folders.filter((folder) => folder.id !== target.id);
      const targetIndex = folders.findIndex((folder) => folder.id === target.id);
      const nextFolder = remaining[Math.min(targetIndex, remaining.length - 1)];
      setFolders(remaining);
      scan.setFolderCount(remaining.length);
      setDeleting(null);
      toast("削除しました。取り込みは自動では始まりません");
      focusFolderAction(nextFolder);
    } catch (failure) {
      await handleFailure(failure, target);
    } finally {
      setPending(null);
    }
  };

  const mutationsDisabled = scan.running;

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-6 sm:py-8">
      <h1 className="text-xl font-semibold">設定</h1>
      <section aria-labelledby="media-folders-heading" className="mt-8">
        <div className="border-b border-border pb-4">
          <h2 id="media-folders-heading" className="text-base font-semibold">
            メディアフォルダ
          </h2>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">
            動画を探すサーバー上のフォルダです。変更後は上部の「ライブラリを更新」から取り込みを実行してください。取り込みは自動では始まりません。
          </p>
          {mutationsDisabled && (
            <p role="status" className="mt-3 text-sm text-warning">
              取り込み中はメディアフォルダを変更できません
            </p>
          )}
        </div>

        <div aria-label="登録済みメディアフォルダ" className="divide-y divide-border">
          {loading && (
            <div
              role="status"
              aria-label="メディアフォルダを読み込み中"
              className="space-y-3 py-5"
            >
              <Skeleton className="h-12" />
              <Skeleton className="h-12" />
            </div>
          )}
          {!loading && loadError !== null && (
            <div className="flex flex-col items-start gap-3 py-6">
              <p role="alert" className="text-sm text-danger">
                メディアフォルダを取得できませんでした: {loadError}
              </p>
              <Button onClick={() => void load()}>再試行</Button>
            </div>
          )}
          {!loading && loadError === null && folders.length === 0 && (
            <div className="py-6">
              <p className="font-medium">メディアフォルダが設定されていません</p>
              <p className="mt-1 text-sm text-fg-muted">
                フォルダを追加すると、手動で取り込めるようになります。
              </p>
            </div>
          )}
          {!loading &&
            loadError === null &&
            folders.map((folder) => {
              const rowPending = pending?.id === folder.id;
              return (
                <div
                  key={folder.id}
                  ref={(element) => {
                    if (element === null) rowRefs.current.delete(folder.id);
                    else rowRefs.current.set(folder.id, element);
                  }}
                  className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center"
                >
                  <div className="flex min-w-0 flex-1 items-start gap-3">
                    <Folder className="mt-0.5 size-4 shrink-0 text-fg-muted" />
                    <code className="min-w-0 break-words text-sm leading-5">
                      {folder.path}
                    </code>
                  </div>
                  <div className="flex shrink-0 items-center justify-end gap-1">
                    {rowPending && (
                      <span role="status" className="mr-2 text-xs text-fg-muted">
                        {pending.kind === "delete" ? "削除中…" : "変更中…"}
                      </span>
                    )}
                    <IconButton
                      label="フォルダを変更"
                      onClick={() => {
                        setOperationError(null);
                        setPicker(folder);
                      }}
                      disabled={rowPending || mutationsDisabled}
                    >
                      <Pencil />
                    </IconButton>
                    <IconButton
                      label="フォルダを削除"
                      onClick={() => {
                        setOperationError(null);
                        setDeleting(folder);
                      }}
                      disabled={rowPending || mutationsDisabled}
                      className="text-danger"
                    >
                      <Trash2 />
                    </IconButton>
                  </div>
                  {rowError?.id === folder.id && (
                    <p role="alert" className="text-sm text-danger sm:basis-full">
                      {rowError.message}
                    </p>
                  )}
                </div>
              );
            })}
        </div>

        <Button
          ref={addButton}
          variant="primary"
          className="mt-5 w-full sm:w-auto"
          onClick={() => {
            setOperationError(null);
            setPicker("add");
          }}
          disabled={
            loading || loadError !== null || mutationsDisabled || pending !== null
          }
          aria-describedby={loadError !== null ? "folder-load-blocked" : undefined}
        >
          <FolderPlus />
          フォルダを追加
        </Button>
        {loadError !== null && (
          <p id="folder-load-blocked" className="mt-2 text-xs text-fg-muted">
            現在値を確認できるまで追加できません
          </p>
        )}
      </section>

      {picker !== null && (
        <FolderPicker
          folders={folders}
          replacing={picker === "add" ? undefined : picker}
          submitting={pending !== null}
          mutationError={operationError}
          onClose={() => {
            if (pending === null) setPicker(null);
          }}
          onSubmit={(path) => void submitFolder(path)}
        />
      )}
      {deleting !== null && (
        <DeleteDialog
          folder={deleting}
          pending={pending !== null}
          error={operationError}
          onClose={() => {
            if (pending === null) setDeleting(null);
          }}
          onDelete={() => void removeFolder()}
        />
      )}
    </div>
  );
}
