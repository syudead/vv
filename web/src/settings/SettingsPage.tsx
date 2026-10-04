import {
  CircleAlert,
  Folder,
  FolderPlus,
  LoaderCircle,
  Pencil,
  Trash2,
} from "lucide-react";
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
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { PageHeader } from "../ui/patterns/page-header";
import { PageSection } from "../ui/patterns/page-section";
import { SettingsPage as SettingsPageLayout } from "../ui/patterns/settings-page";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "../ui/shadcn/alert-dialog";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { useToast } from "../ui/Toast";
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
  // 確認の窓の型（ConfirmDialog）と同じ組み方で、削除が終わるまで窓を開いたままにし、
  // 失敗を窓の中に出す。
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t.settings.removeDialog.title}</AlertDialogTitle>
          <AlertDialogDescription>
            {t.settings.removeDialog.warning}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="min-w-0 rounded-md bg-muted p-3">
          <p className="text-xs text-muted-foreground">
            {t.settings.removeDialog.target}
          </p>
          <code className="block text-sm break-all">{folder.path}</code>
        </div>
        {error !== null && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{error}</AlertTitle>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t.common.cancel}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            onClick={(event) => {
              event.preventDefault();
              onDelete();
            }}
          >
            {pending && (
              <LoaderCircle className="animate-spin motion-reduce:animate-none" />
            )}
            {pending ? t.settings.removeDialog.removing : t.settings.removeDialog.submit}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
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

  const text = t.settings.mediaFolders;

  return (
    <SettingsPageLayout header={<PageHeader title={t.settings.title} />}>
      <ScanStatusSection />
      <PageSection
        title={text.heading}
        description={text.description}
        actions={
          <Button
            ref={addButton}
            variant="outline"
            size="sm"
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
            {text.add}
          </Button>
        }
      >
        {mutationsDisabled && (
          <p role="status" className="text-warning">
            {text.lockedWhileScanning}
          </p>
        )}
        {loadError !== null && (
          <p id="folder-load-blocked" className="text-xs text-muted-foreground">
            {text.addBlocked}
          </p>
        )}
        <div aria-label={text.list} className="flex flex-col divide-y divide-border">
          {loading && (
            <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
              <Skeleton className="h-10" />
              <Skeleton className="h-10" />
            </div>
          )}
          {!loading && loadError !== null && (
            <ErrorState
              title={text.loadFailed(loadError)}
              retryLabel={t.common.retry}
              onRetry={() => void load()}
            />
          )}
          {!loading && loadError === null && folders.length === 0 && (
            <EmptyState
              icon={<Folder />}
              title={text.empty}
              description={text.emptyHint}
            />
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
                  className="flex min-w-0 flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:flex-wrap sm:items-center"
                >
                  <div className="flex min-w-0 flex-1 items-start gap-3">
                    <Folder className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                    <div className="flex min-w-0 flex-1 flex-col">
                      <span className="text-xs text-muted-foreground">
                        {text.current}
                      </span>
                      <code className="block break-all" title={folder.path}>
                        {folder.path}
                      </code>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                    {rowPending && (
                      <span role="status" className="text-xs text-muted-foreground">
                        {pending.kind === "delete" ? text.removing : text.changing}
                      </span>
                    )}
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={text.change}
                      onClick={() => {
                        setOperationError(null);
                        setPicker(folder);
                      }}
                      disabled={rowPending || mutationsDisabled}
                    >
                      <Pencil />
                      {text.changeShort}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label={text.remove}
                      onClick={() => {
                        setOperationError(null);
                        setDeleting(folder);
                      }}
                      disabled={rowPending || mutationsDisabled}
                      className="text-destructive"
                    >
                      <Trash2 />
                      {text.removeShort}
                    </Button>
                  </div>
                  {rowError?.id === folder.id && (
                    <p role="alert" className="text-destructive sm:basis-full">
                      {rowError.message}
                    </p>
                  )}
                </div>
              );
            })}
        </div>
      </PageSection>
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
    </SettingsPageLayout>
  );
}
