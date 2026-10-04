import { Folder, FolderPlus, LoaderCircle, Pencil, Trash2 } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  createMediaFolder,
  deleteMediaFolder,
  listMediaFolders,
  RequestFailed,
  updateMediaFolder,
  type MediaFolder,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { useScan } from "../shell/ScanProvider";
import Button from "../ui/Button";
import { ModalFrame } from "../ui/legacy/ModalFrame";
import Skeleton from "../ui/legacy/Skeleton";
import { useToast } from "../ui/legacy/Toast";
import FolderPicker from "./FolderPicker";
import NetworkSection from "./NetworkSection";
import APITokensSection from "./APITokensSection";
import ScanStatusSection from "./ScanStatusSection";
import TranscodingSection from "./TranscodingSection";

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
  error: UiText | null;
  onClose: () => void;
  onDelete: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  return (
    <ModalFrame
      title={t.settings.removeDialog.title}
      onClose={onClose}
      initialFocus={cancel}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4 sm:p-5">
        <div className="min-w-0 rounded-md border border-control-border bg-field p-3">
          <p className="mb-1 text-xs text-fg-muted">{t.settings.removeDialog.target}</p>
          <code className="block break-all text-sm text-fg">{folder.path}</code>
        </div>
        <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
          {t.settings.removeDialog.warning}
        </p>
        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-border p-4">
        <Button ref={cancel} onClick={onClose} disabled={pending}>
          {t.common.cancel}
        </Button>
        <Button variant="danger" onClick={onDelete} disabled={pending}>
          {pending && <LoaderCircle className="animate-spin" />}
          {pending ? t.settings.removeDialog.removing : t.settings.removeDialog.submit}
        </Button>
      </div>
    </ModalFrame>
  );
}

export default function SettingsPage() {
  const scan = useScan();
  const { setFolderCount } = scan;
  const toast = useToast();
  const [folders, setFolders] = useState<MediaFolder[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [picker, setPicker] = useState<MediaFolder | "add" | null>(null);
  const [deleting, setDeleting] = useState<MediaFolder | null>(null);
  const [pending, setPending] = useState<Pending>(null);
  const [operationError, setOperationError] = useState<UiText | null>(null);
  const [rowError, setRowError] = useState<{ id: number; message: UiText } | null>(null);
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
        setFolderCount(result.length);
        return sorted;
      } catch (failure) {
        if (signal?.aborted) return;
        setLoadError(errorText(failure));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [setFolderCount],
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
        message: t.settings.mediaFolders.changedElsewhere,
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
      setOperationError(t.settings.mediaFolders.lockedWhileScanning);
      scan.refresh();
      return;
    }
    setOperationError(errorText(failure));
  };

  const submitFolder = async (path: string) => {
    const replacing = picker === "add" ? undefined : (picker ?? undefined);
    setOperationError(null);
    setPending({ id: replacing?.id ?? "new", kind: replacing ? "change" : "add" });
    try {
      if (replacing === undefined) {
        const created = await createMediaFolder(path);
        setFolders((current) => [...current, created].sort((a, b) => a.id - b.id));
        scan.setFolderCount(1);
        await load();
        toast(t.settings.mediaFolders.added);
      } else {
        const updated = await updateMediaFolder(replacing.id, path, replacing.version);
        setFolders((current) =>
          current.map((folder) => (folder.id === updated.id ? updated : folder)),
        );
        setRowError((current) => (current?.id === updated.id ? null : current));
        toast(t.settings.mediaFolders.changed);
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
    const targetIndex = folders.findIndex((folder) => folder.id === target.id);
    setOperationError(null);
    setPending({ id: target.id, kind: "delete" });
    try {
      await deleteMediaFolder(target.id, target.version);
      const remaining = folders.filter((folder) => folder.id !== target.id);
      setFolders(remaining);
      scan.setFolderCount(remaining.length);
      setDeleting(null);
      toast(t.settings.mediaFolders.removed);
      const refreshed = await load();
      const current = refreshed ?? remaining;
      const nextFolder = current[Math.min(targetIndex, current.length - 1)];
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
      <h1 className="text-xl font-semibold">{t.settings.title}</h1>
      <ScanStatusSection />
      <section
        aria-labelledby="media-folders-heading"
        className="mt-8 rounded-lg border border-border bg-surface p-4 sm:p-5"
      >
        <div className="border-b border-border pb-4">
          <h2 id="media-folders-heading" className="text-base font-semibold">
            {t.settings.mediaFolders.heading}
          </h2>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">
            {t.settings.mediaFolders.description}
          </p>
          {mutationsDisabled && (
            <p role="status" className="mt-3 text-sm text-warning">
              {t.settings.mediaFolders.lockedWhileScanning}
            </p>
          )}
        </div>

        <div aria-label={t.settings.mediaFolders.list} className="divide-y divide-border">
          {loading && (
            <div
              role="status"
              aria-label={t.settings.mediaFolders.loading}
              className="space-y-3 py-5"
            >
              <Skeleton className="h-12" />
              <Skeleton className="h-12" />
            </div>
          )}
          {!loading && loadError !== null && (
            <div className="flex flex-col items-start gap-3 py-6">
              <p role="alert" className="text-sm text-danger">
                {t.settings.mediaFolders.loadFailed(loadError)}
              </p>
              <Button onClick={() => void load()}>{t.common.retry}</Button>
            </div>
          )}
          {!loading && loadError === null && folders.length === 0 && (
            <div className="py-6">
              <p className="font-medium">{t.settings.mediaFolders.empty}</p>
              <p className="mt-1 text-sm text-fg-muted">
                {t.settings.mediaFolders.emptyHint}
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
                  className="flex min-w-0 flex-col gap-3 py-4 sm:flex-row sm:items-center"
                >
                  <div className="flex min-w-0 flex-1 items-start gap-3">
                    <Folder className="mt-2 size-4 shrink-0 text-fg-muted" />
                    <div className="min-w-0 flex-1 rounded-md border border-control-border bg-field px-3 py-2">
                      <span className="block text-xs text-fg-muted">
                        {t.settings.mediaFolders.current}
                      </span>
                      <code
                        className="block break-all text-sm leading-5"
                        title={folder.path}
                      >
                        {folder.path}
                      </code>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                    {rowPending && (
                      <span role="status" className="mr-2 text-xs text-fg-muted">
                        {pending.kind === "delete"
                          ? t.settings.mediaFolders.removing
                          : t.settings.mediaFolders.changing}
                      </span>
                    )}
                    <Button
                      size="sm"
                      aria-label={t.settings.mediaFolders.change}
                      onClick={() => {
                        setOperationError(null);
                        setPicker(folder);
                      }}
                      disabled={rowPending || mutationsDisabled}
                    >
                      <Pencil />
                      {t.settings.mediaFolders.changeShort}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label={t.settings.mediaFolders.remove}
                      onClick={() => {
                        setOperationError(null);
                        setDeleting(folder);
                      }}
                      disabled={rowPending || mutationsDisabled}
                      className="text-danger"
                    >
                      <Trash2 />
                      {t.settings.mediaFolders.removeShort}
                    </Button>
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
          {t.settings.mediaFolders.add}
        </Button>
        {loadError !== null && (
          <p id="folder-load-blocked" className="mt-2 text-xs text-fg-muted">
            {t.settings.mediaFolders.addBlocked}
          </p>
        )}
      </section>
      <TranscodingSection />
      <APITokensSection />
      <NetworkSection />

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
