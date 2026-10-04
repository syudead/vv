import {
  ArrowLeft,
  ArrowUp,
  ChevronRight,
  CircleAlert,
  Folder,
  LoaderCircle,
} from "lucide-react";
import { type KeyboardEvent, useCallback, useEffect, useRef, useState } from "react";

import { listDirectories, type DirectoryListing, type MediaFolder } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/shadcn/dialog";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

function normalizedForComparison(path: string): string {
  const slash = path.replaceAll("\\", "/");
  const trimmed = slash.length > 1 ? slash.replace(/\/+$/, "") : slash;
  return /^[a-z]:/i.test(trimmed) ? trimmed.toLocaleLowerCase() : trimmed;
}

function pathsOverlap(left: string, right: string): boolean {
  const a = normalizedForComparison(left);
  const b = normalizedForComparison(right);
  if (a === b) return true;
  const prefix = (value: string) => (value.endsWith("/") ? value : `${value}/`);
  return a.startsWith(prefix(b)) || b.startsWith(prefix(a));
}

function candidateProblem(
  listing: DirectoryListing | null,
  folders: MediaFolder[],
  replacing?: MediaFolder,
): UiText | null {
  const text = t.settings.picker;
  const path = listing?.currentPath;
  if (path === undefined) return text.selectFolder;
  if (replacing?.path === path) return text.sameFolder;
  const conflict = folders.find(
    (folder) => folder.id !== replacing?.id && pathsOverlap(folder.path, path),
  );
  return conflict === undefined ? null : text.overlaps;
}

export default function FolderPicker({
  folders,
  replacing,
  submitting,
  mutationError,
  onClose,
  onSubmit,
}: {
  folders: MediaFolder[];
  replacing?: MediaFolder;
  submitting: boolean;
  mutationError: UiText | null;
  onClose: () => void;
  onSubmit: (path: string) => void;
}) {
  const [listing, setListing] = useState<DirectoryListing | null>(null);
  const [requestedPath, setRequestedPath] = useState<string | undefined>(replacing?.path);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const rows = useRef<Array<HTMLButtonElement | null>>([]);
  const listRegion = useRef<HTMLDivElement>(null);
  const confirmBack = useRef<HTMLButtonElement>(null);
  const loadController = useRef<AbortController | null>(null);
  const focusAfterLoad = useRef(false);

  const load = useCallback((path?: string, restoreKeyboardFocus = false) => {
    loadController.current?.abort();
    focusAfterLoad.current = restoreKeyboardFocus;
    setRequestedPath(path);
    setLoading(true);
    setLoadError(null);
    const controller = new AbortController();
    loadController.current = controller;
    void listDirectories(path, controller.signal)
      .then((value) => {
        setListing(value);
        setActiveIndex(0);
      })
      .catch((failure) => {
        if (!controller.signal.aborted) setLoadError(errorText(failure));
      })
      .finally(() => {
        if (loadController.current === controller) setLoading(false);
      });
  }, []);

  useEffect(() => {
    load(replacing?.path);
    return () => loadController.current?.abort();
  }, [load, replacing?.path]);

  useEffect(() => {
    if (loading || listing === null || !focusAfterLoad.current) return;
    focusAfterLoad.current = false;
    (rows.current[0] ?? listRegion.current)?.focus();
  }, [listing, loading]);

  const onListKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (listing === null || listing.directories.length === 0) return;
    let next = activeIndex;
    if (event.key === "ArrowDown")
      next = Math.min(activeIndex + 1, listing.directories.length - 1);
    else if (event.key === "ArrowUp") next = Math.max(activeIndex - 1, 0);
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = listing.directories.length - 1;
    else if (event.key === "ArrowLeft" && listing.parentPath !== null) {
      event.preventDefault();
      load(listing.parentPath ?? undefined, true);
      return;
    } else return;
    event.preventDefault();
    setActiveIndex(next);
    rows.current[next]?.focus();
  };

  // 確かめの段に移ったら、戻る操作へ focus を移す。窓は開いたままなので Radix は移さない。
  useEffect(() => {
    if (confirming) confirmBack.current?.focus();
  }, [confirming]);

  const problem = candidateProblem(listing, folders, replacing);
  const currentPath = listing?.currentPath;
  const text = t.settings.picker;
  const title = replacing ? text.changeTitle : text.addTitle;

  const showConfirm = confirming && replacing !== undefined && currentPath !== undefined;

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{showConfirm ? text.confirmTitle : title}</DialogTitle>
        </DialogHeader>
        {showConfirm ? (
          <>
            <div className="grid min-w-0 gap-3">
              <div className="min-w-0 rounded-md bg-muted p-3">
                <span className="block text-xs font-medium text-muted-foreground">
                  {text.before}
                </span>
                <code className="block min-w-0 text-sm break-all">{replacing.path}</code>
              </div>
              <div className="min-w-0 rounded-md bg-muted p-3">
                <span className="block text-xs font-medium text-muted-foreground">
                  {text.after}
                </span>
                <code className="block min-w-0 text-sm break-all">{currentPath}</code>
              </div>
            </div>
            <Alert variant="warning" role="note">
              <CircleAlert aria-hidden="true" />
              <AlertDescription>{text.confirmWarning}</AlertDescription>
            </Alert>
            {mutationError !== null && (
              <Alert variant="destructive">
                <CircleAlert aria-hidden="true" />
                <AlertTitle>{mutationError}</AlertTitle>
              </Alert>
            )}
            <DialogFooter>
              <Button
                ref={confirmBack}
                variant="outline"
                size="sm"
                onClick={() => setConfirming(false)}
                disabled={submitting}
              >
                <ArrowLeft />
                {t.common.back}
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => onSubmit(currentPath)}
                disabled={submitting}
              >
                {submitting && (
                  <LoaderCircle className="animate-spin motion-reduce:animate-none" />
                )}
                {submitting ? text.changing : text.confirm}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <div className="flex min-w-0 items-start gap-2 rounded-md bg-muted p-2">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={text.parent}
                    onClick={(event) =>
                      load(listing?.parentPath ?? undefined, event.detail === 0)
                    }
                    disabled={loading || listing?.parentPath == null}
                  >
                    <ArrowUp />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{text.parent}</TooltipContent>
              </Tooltip>
              <code
                aria-live="polite"
                className="min-w-0 flex-1 pt-1.5 text-xs break-all text-muted-foreground"
              >
                {listing?.currentPath ?? text.fileSystem}
              </code>
            </div>

            <div
              ref={listRegion}
              tabIndex={-1}
              className="h-folder-list overflow-y-auto rounded-md border border-border p-1"
              onKeyDown={onListKeyDown}
              aria-label={text.list}
            >
              {loading && (
                <div
                  role="status"
                  aria-label={text.loading}
                  className="flex flex-col gap-1"
                >
                  <Skeleton className="h-10" />
                  <Skeleton className="h-10" />
                  <Skeleton className="h-10" />
                </div>
              )}
              {!loading && loadError !== null && (
                <div className="flex flex-col gap-2 p-2">
                  <ErrorState
                    title={loadError}
                    retryLabel={t.common.retry}
                    onRetry={() => load(requestedPath)}
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    className="self-start"
                    onClick={() => load()}
                  >
                    {text.toRoot}
                  </Button>
                </div>
              )}
              {!loading && loadError === null && listing?.directories.length === 0 && (
                <p className="p-6 text-center text-sm text-muted-foreground">
                  {text.noSubfolders}
                </p>
              )}
              {!loading && loadError === null && listing !== null && (
                <div role="list">
                  {listing.directories.map((directory, index) => (
                    <div key={directory.path} role="listitem">
                      <Button
                        ref={(element) => {
                          rows.current[index] = element;
                        }}
                        variant="ghost"
                        tabIndex={index === activeIndex ? 0 : -1}
                        onFocus={() => setActiveIndex(index)}
                        onClick={(event) => load(directory.path, event.detail === 0)}
                        onKeyDown={(event) => {
                          if (event.key === "ArrowRight") {
                            event.preventDefault();
                            load(directory.path, true);
                          }
                        }}
                        className="h-auto min-h-10 w-full min-w-0 justify-start gap-3 py-2 text-left font-normal whitespace-normal focus:bg-accent"
                      >
                        <Folder className="text-muted-foreground" />
                        <span className="min-w-0 flex-1 break-all">{directory.name}</span>
                        <ChevronRight className="text-muted-foreground" />
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {!loading && loadError === null && problem !== null && (
              <p className="text-xs text-warning" role="status">
                {problem}
              </p>
            )}
            {mutationError !== null && (
              <Alert variant="destructive">
                <CircleAlert aria-hidden="true" />
                <AlertTitle>{mutationError}</AlertTitle>
              </Alert>
            )}
            <DialogFooter>
              <Button variant="outline" size="sm" onClick={onClose} disabled={submitting}>
                {t.common.cancel}
              </Button>
              <Button
                size="sm"
                disabled={loading || loadError !== null || problem !== null || submitting}
                onClick={() => {
                  if (currentPath === undefined) return;
                  if (replacing === undefined) onSubmit(currentPath);
                  else setConfirming(true);
                }}
              >
                {submitting && (
                  <LoaderCircle className="animate-spin motion-reduce:animate-none" />
                )}
                {submitting
                  ? replacing
                    ? text.changing
                    : text.adding
                  : replacing
                    ? text.changeToThis
                    : text.addThis}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
