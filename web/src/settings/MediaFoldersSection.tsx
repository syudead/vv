import {
  CircleAlert,
  Folder,
  FolderPlus,
  LoaderCircle,
  Pencil,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  createMediaFolder,
  deleteMediaFolder,
  listMediaFolders,
  RequestFailed,
  setMediaFolderEnabled,
  updateMediaFolder,
  type MediaFolder,
} from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { useScan } from "../shell/ScanProvider";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
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
import { Label } from "../ui/shadcn/label";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Switch } from "../ui/shadcn/switch";
import { useToast } from "../ui/Toast";
import FolderPicker from "./FolderPicker";
import MediaFolderUsage from "./MediaFolderUsage";
import RescanDialog from "./RescanDialog";
import { useSwitchSave } from "./useSwitchSave";

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
  // Keep the dialog open during deletion and show failures inside it.
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

/**
 * The switch stays usable while a scan locks the three buttons, since a switch changes no registered folder.
 * Its failure takes the row's failure line, where a change made elsewhere is reported.
 */
function LocalFolderRow({
  folder,
  pending,
  locked,
  error,
  rowRef,
  onChange,
  onRescan,
  onRemove,
  onSwitched,
}: {
  folder: MediaFolder;
  pending: NonNullable<Pending>["kind"] | null;
  locked: boolean;
  error: UiText | null;
  rowRef: (element: HTMLDivElement | null) => void;
  onChange: () => void;
  onRescan: () => void;
  onRemove: () => void;
  onSwitched: (folder: MediaFolder) => void;
}) {
  const text = t.settings.mediaFolders;
  const { saving, saveError, toggle } = useSwitchSave({
    settings: folder,
    current: (saved) => saved.enabled,
    save: (next) => setMediaFolderEnabled(folder.id, next),
    saved: onSwitched,
    saveFailed: text.switchFailed,
  });
  const enabled = saving ?? folder.enabled;
  const disabled = pending !== null || locked;
  const failure = saveError ?? error;
  return (
    <div
      ref={rowRef}
      className="flex min-w-0 flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:flex-wrap sm:items-center"
    >
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <Folder
          className={cn(
            "mt-0.5 size-4 shrink-0 text-muted-foreground",
            !enabled && "opacity-50",
          )}
        />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="text-xs text-muted-foreground">{text.current}</span>
          <code
            className={cn("block break-all", !enabled && "text-muted-foreground")}
            title={folder.path}
          >
            {folder.path}
          </code>
          <MediaFolderUsage folder={folder} />
        </div>
      </div>
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
        {pending !== null && (
          <span role="status" className="text-xs text-muted-foreground">
            {pending === "delete" ? text.removing : text.changing}
          </span>
        )}
        <Button
          variant="outline"
          size="sm"
          aria-label={text.change}
          onClick={onChange}
          disabled={disabled}
        >
          <Pencil />
          {text.changeShort}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-label={text.forceRescan}
          aria-describedby={locked ? "media-folders-locked" : undefined}
          onClick={onRescan}
          disabled={disabled}
        >
          <RotateCcw />
          {text.forceRescanShort}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-label={text.remove}
          onClick={onRemove}
          disabled={disabled}
          className="text-destructive"
        >
          <Trash2 />
          {text.removeShort}
        </Button>
        {/* The word and the track are one control in a fixed column, so every switch of the page lines up. */}
        <Label className="h-8 w-16 shrink-0 cursor-pointer justify-end text-xs font-normal text-muted-foreground">
          {enabled ? text.on : text.off}
          <Switch
            checked={enabled}
            aria-label={text.switchLabel(folder.path)}
            aria-disabled={saving !== null || undefined}
            onCheckedChange={() => void toggle()}
          />
        </Label>
      </div>
      {failure !== null && (
        <p role="alert" className="text-destructive sm:basis-full">
          {failure}
        </p>
      )}
    </div>
  );
}

/** Report media-folder counts to ScanProvider so newly added folders enable Scan library. */
export default function MediaFoldersSection() {
  const scan = useScan();
  const { setFolderCount } = scan;
  const toast = useToast();
  const [folders, setFolders] = useState<MediaFolder[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [picker, setPicker] = useState<MediaFolder | "add" | null>(null);
  const [deleting, setDeleting] = useState<MediaFolder | null>(null);
  const [rescanning, setRescanning] = useState<MediaFolder | null>(null);
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

  // A change made elsewhere replaces the rows in place: no skeleton, and a failed read keeps the rows shown.
  useEffect(() => {
    const controller = new AbortController();
    const reread = async () => {
      try {
        const result = await listMediaFolders(controller.signal);
        setFolders([...result].sort((a, b) => a.id - b.id));
        setFolderCount(result.length);
      } catch {
        // The next event or a reload reads again.
      }
    };
    const unsubscribe = subscribeServerEvents({ mediaFolders: () => void reread() });
    return () => {
      unsubscribe();
      controller.abort();
    };
  }, [setFolderCount]);

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
    <>
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
          <p id="media-folders-locked" role="status" className="text-warning">
            {text.rowsLockedWhileScanning}
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
            folders.map((folder) => (
              <LocalFolderRow
                key={folder.id}
                folder={folder}
                pending={pending?.id === folder.id ? pending.kind : null}
                locked={mutationsDisabled}
                error={rowError?.id === folder.id ? rowError.message : null}
                rowRef={(element) => {
                  if (element === null) rowRefs.current.delete(folder.id);
                  else rowRefs.current.set(folder.id, element);
                }}
                onChange={() => {
                  setOperationError(null);
                  setPicker(folder);
                }}
                onRescan={() => setRescanning(folder)}
                onRemove={() => {
                  setOperationError(null);
                  setDeleting(folder);
                }}
                onSwitched={(switched) =>
                  setFolders((current) =>
                    current.map((item) => (item.id === switched.id ? switched : item)),
                  )
                }
              />
            ))}
        </div>
      </PageSection>
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
      {rescanning !== null && (
        <RescanDialog
          request={{ scope: "folder", folderId: rescanning.id }}
          title={text.rescanDialog.title}
          description={text.rescanDialog.description}
          onClose={() => setRescanning(null)}
        >
          <div className="min-w-0 rounded-md bg-muted p-3">
            <p className="text-xs text-muted-foreground">{text.rescanDialog.target}</p>
            <code className="block text-sm break-all">{rescanning.path}</code>
          </div>
        </RescanDialog>
      )}
    </>
  );
}
