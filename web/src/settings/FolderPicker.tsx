import { ArrowLeft, ArrowUp, ChevronRight, Folder, LoaderCircle, X } from "lucide-react";
import {
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";

import {
  errorMessage,
  listDirectories,
  type DirectoryListing,
  type MediaFolder,
} from "../api/client";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import Skeleton from "../ui/Skeleton";

function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => !element.hasAttribute("hidden"));
}

export function ModalFrame({
  title,
  onClose,
  children,
  initialFocus,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  initialFocus?: RefObject<HTMLElement | null>;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const overlay = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const titleId = `dialog-${title.replaceAll(" ", "-")}`;

  useEffect(() => {
    const previous =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const background = Array.from(document.body.children).filter(
      (element) => element !== overlay.current,
    );
    const states = background.map((element) => ({
      element,
      inert: element.hasAttribute("inert"),
      ariaHidden: element.getAttribute("aria-hidden"),
    }));
    for (const element of background) {
      element.setAttribute("inert", "");
      element.setAttribute("aria-hidden", "true");
    }
    const target = initialFocus?.current ?? focusableElements(panel.current!)[0];
    target?.focus();
    const keydown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      if (event.key !== "Tab" || panel.current === null) return;
      const elements = focusableElements(panel.current);
      if (elements.length === 0) return;
      const first = elements[0]!;
      const last = elements.at(-1)!;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", keydown);
    return () => {
      document.removeEventListener("keydown", keydown);
      for (const state of states) {
        state.element.toggleAttribute("inert", state.inert);
        if (state.ariaHidden === null) state.element.removeAttribute("aria-hidden");
        else state.element.setAttribute("aria-hidden", state.ariaHidden);
      }
      queueMicrotask(() => previous?.focus());
    };
  }, [initialFocus]);

  return createPortal(
    <div
      ref={overlay}
      className="fixed inset-0 z-50 flex items-stretch justify-center bg-overlay sm:items-center sm:p-6"
    >
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex min-h-0 w-full flex-col bg-elevated shadow-elevated sm:max-h-[calc(100dvh-3rem)] sm:max-w-2xl sm:rounded-lg sm:border sm:border-border-strong"
      >
        <div className="flex h-14 shrink-0 items-center gap-3 border-b border-border px-4">
          <h2 id={titleId} className="min-w-0 flex-1 truncate text-base font-semibold">
            {title}
          </h2>
          <IconButton label="閉じる" onClick={onClose} tooltip={false}>
            <X />
          </IconButton>
        </div>
        {children}
      </div>
    </div>,
    document.body,
  );
}

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
): string | null {
  const path = listing?.currentPath;
  if (path === undefined || listing?.parentPath === null) {
    return "ファイルシステムまたはドライブのルートは選択できません";
  }
  if (replacing?.path === path) return "現在と同じフォルダです";
  const conflict = folders.find(
    (folder) => folder.id !== replacing?.id && pathsOverlap(folder.path, path),
  );
  return conflict === undefined
    ? null
    : "登録済みフォルダと同じ場所、またはその親子は選択できません";
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
  mutationError: string | null;
  onClose: () => void;
  onSubmit: (path: string) => void;
}) {
  const [listing, setListing] = useState<DirectoryListing | null>(null);
  const [requestedPath, setRequestedPath] = useState<string | undefined>(replacing?.path);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const rows = useRef<Array<HTMLButtonElement | null>>([]);
  const listRegion = useRef<HTMLDivElement>(null);
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
        if (!controller.signal.aborted) setLoadError(errorMessage(failure));
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

  const problem = candidateProblem(listing, folders, replacing);
  const currentPath = listing?.currentPath;
  const title = replacing ? "メディアフォルダを変更" : "メディアフォルダを追加";

  if (confirming && replacing !== undefined && currentPath !== undefined) {
    return (
      <ModalFrame title="フォルダの変更を確認" onClose={onClose}>
        <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4 sm:p-5">
          <div className="grid gap-4 sm:grid-cols-[auto_1fr]">
            <span className="text-xs font-medium text-fg-muted">変更前</span>
            <code className="min-w-0 break-words text-sm text-fg">{replacing.path}</code>
            <span className="text-xs font-medium text-fg-muted">変更後</span>
            <code className="min-w-0 break-words text-sm text-fg">{currentPath}</code>
          </div>
          <p className="border-l-2 border-warning-strong pl-3 text-sm leading-6 text-fg-muted">
            変更前のフォルダだけにある動画は一覧から外れます。別の登録フォルダにもある動画は残ります。再生位置と視聴済み状態は残ります。取り込みは自動では始まりません。
          </p>
          {mutationError !== null && (
            <p role="alert" className="text-sm text-danger">
              {mutationError}
            </p>
          )}
        </div>
        <div className="flex shrink-0 justify-end gap-2 border-t border-border p-4">
          <Button onClick={() => setConfirming(false)} disabled={submitting}>
            <ArrowLeft />
            戻る
          </Button>
          <Button
            variant="danger"
            onClick={() => onSubmit(currentPath)}
            disabled={submitting}
          >
            {submitting && <LoaderCircle className="animate-spin" />}
            変更する
          </Button>
        </div>
      </ModalFrame>
    );
  }

  return (
    <ModalFrame title={title} onClose={onClose}>
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="flex shrink-0 items-start gap-2 border-b border-border px-4 py-3">
          <IconButton
            label="親フォルダへ戻る"
            onClick={(event) =>
              load(listing?.parentPath ?? undefined, event.detail === 0)
            }
            disabled={loading || listing?.parentPath == null}
          >
            <ArrowUp />
          </IconButton>
          <code
            aria-live="polite"
            className="min-w-0 flex-1 break-words pt-2 text-xs leading-5 text-fg-muted"
          >
            {listing?.currentPath ?? "ファイルシステム"}
          </code>
        </div>

        <div
          ref={listRegion}
          tabIndex={-1}
          className="min-h-0 flex-1 overflow-y-auto p-2 sm:min-h-72"
          onKeyDown={onListKeyDown}
          aria-label="フォルダ一覧"
        >
          {loading && (
            <div
              role="status"
              aria-label="フォルダを読み込み中"
              className="space-y-2 p-2"
            >
              <Skeleton className="h-10" />
              <Skeleton className="h-10" />
              <Skeleton className="h-10" />
            </div>
          )}
          {!loading && loadError !== null && (
            <div className="flex min-h-48 flex-col items-center justify-center gap-3 p-4 text-center">
              <p role="alert" className="text-sm text-danger">
                {loadError}
              </p>
              <div className="flex flex-wrap justify-center gap-2">
                <Button onClick={() => load(requestedPath)}>再試行</Button>
                <Button variant="ghost" onClick={() => load()}>
                  ルートへ戻る
                </Button>
              </div>
            </div>
          )}
          {!loading && loadError === null && listing?.directories.length === 0 && (
            <p className="p-6 text-center text-sm text-fg-muted">
              子フォルダはありません
            </p>
          )}
          {!loading && loadError === null && listing !== null && (
            <div role="list" className="divide-y divide-border">
              {listing.directories.map((directory, index) => (
                <div key={directory.path} role="listitem">
                  <button
                    ref={(element) => {
                      rows.current[index] = element;
                    }}
                    type="button"
                    tabIndex={index === activeIndex ? 0 : -1}
                    onFocus={() => setActiveIndex(index)}
                    onClick={(event) => load(directory.path, event.detail === 0)}
                    onKeyDown={(event) => {
                      if (event.key === "ArrowRight") {
                        event.preventDefault();
                        load(directory.path, true);
                      }
                    }}
                    className="flex min-h-11 w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-hover-wash focus:bg-active-wash"
                  >
                    <Folder className="size-4 shrink-0 text-fg-muted" />
                    <span className="min-w-0 flex-1 break-words">{directory.name}</span>
                    <ChevronRight className="size-4 shrink-0 text-fg-subtle" />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="shrink-0 border-t border-border p-4">
          {!loading && loadError === null && problem !== null && (
            <p className="mb-3 text-xs text-warning" role="status">
              {problem}
            </p>
          )}
          {mutationError !== null && (
            <p role="alert" className="mb-3 text-sm text-danger">
              {mutationError}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={onClose} disabled={submitting}>
              キャンセル
            </Button>
            <Button
              variant="primary"
              disabled={loading || loadError !== null || problem !== null || submitting}
              onClick={() => {
                if (currentPath === undefined) return;
                if (replacing === undefined) onSubmit(currentPath);
                else setConfirming(true);
              }}
            >
              {submitting && <LoaderCircle className="animate-spin" />}
              {replacing ? "このフォルダに変更" : "このフォルダを追加"}
            </Button>
          </div>
        </div>
      </div>
    </ModalFrame>
  );
}
