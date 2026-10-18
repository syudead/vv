import { type RefObject, useCallback, useEffect, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  createTagKind,
  currentTagKinds,
  deleteTagKind,
  refreshTagKinds,
  renameTagKind,
  sortTagKinds,
  subscribeTagKinds,
  type TagKind,
} from "../api/tagKinds";
import { errorText, t, type UiText } from "../i18n";
import type { ShowToast } from "../ui/Toast";
import { kindFieldError, type TagFieldError } from "./tagNameField";
import { isShown } from "./useVirtualTagRows";

export interface KindRowRefs {
  nameLink: HTMLAnchorElement | null;
  renameButton: HTMLButtonElement | null;
  menuButton: HTMLButtonElement | null;
  actionsButton: HTMLButtonElement | null;
}

export function withKind(kinds: readonly TagKind[], kind: TagKind): TagKind[] {
  return sortTagKinds([...kinds.filter((item) => item.id !== kind.id), kind]);
}

export function nextAfterRemoval(
  order: readonly TagKind[],
  removedId: number,
): TagKind | undefined {
  const index = order.findIndex((item) => item.id === removedId);
  if (index < 0) return undefined;
  return order[index + 1] ?? order[index - 1];
}

function isKindNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.reason === "tag_kind_not_found";
}

export interface TagKindsTab {
  kinds: TagKind[] | undefined;
  /** Cached data alone cannot prove a URL kind is gone; require a successful read since mount. */
  fresh: boolean;
  loadError: UiText | null;
  creating: boolean;
  createPending: boolean;
  createError: TagFieldError | null;
  renamingId: number | null;
  renamePending: boolean;
  renameError: TagFieldError | null;
  deleting: TagKind | null;
  deletePending: boolean;
  deleteError: UiText | null;
  pending: boolean;
  editing: boolean;
  registerRefs: (id: number, refs: Partial<KindRowRefs>) => void;
  reload: () => void;
  openCreate: () => void;
  cancelCreate: () => void;
  submitCreate: (name: string) => void;
  clearCreateError: () => void;
  startRename: (kind: TagKind) => void;
  cancelRename: (kind: TagKind) => void;
  submitRename: (kind: TagKind, name: string) => void;
  clearRenameError: () => void;
  openDelete: (kind: TagKind) => void;
  cancelDelete: () => void;
  performDelete: (kind: TagKind) => void;
  closeEdits: () => void;
  /** Consume rename/deletion changes to decide whether the Tags tab must refresh. */
  takeChanged: () => boolean;
}

/** Refresh on entry even with cached kinds because counts may have changed; apply edits locally while the API refreshes shared state. */
export function useTagKindsTab({
  toast,
  newKindRef,
}: {
  toast: ShowToast;
  newKindRef: RefObject<HTMLButtonElement | null>;
}): TagKindsTab {
  const [kinds, setKinds] = useState<TagKind[] | undefined>(() => {
    const held = currentTagKinds();
    return held === undefined ? undefined : sortTagKinds(held);
  });
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [fresh, setFresh] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);
  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);
  const [deleting, setDeleting] = useState<TagKind | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);
  const changed = useRef(false);
  const rowRefs = useRef(new Map<number, Partial<KindRowRefs>>());

  useEffect(() => {
    const received = (list: TagKind[]) => {
      setKinds(sortTagKinds(list));
      setLoadError(null);
      setFresh(true);
    };
    const unsubscribe = subscribeTagKinds(received);
    refreshTagKinds().then(received, (failure: unknown) =>
      setLoadError(errorText(failure)),
    );
    return unsubscribe;
  }, []);

  /** Preserve rows during refresh, but expose failure instead of stale counts used by deletion confirmation. */
  const reload = useCallback(() => {
    setLoadError(null);
    refreshTagKinds().catch((failure: unknown) => setLoadError(errorText(failure)));
  }, []);

  const registerRefs = useCallback((id: number, refs: Partial<KindRowRefs>) => {
    const current = rowRefs.current.get(id) ?? {};
    rowRefs.current.set(id, { ...current, ...refs });
  }, []);

  /** Focus compact Actions when individual buttons are hidden; fall back to New kind for missing rows. */
  function focusLater(id: number | undefined, part: "name" | "rename" | "menu") {
    setTimeout(() => {
      const refs = id === undefined ? undefined : rowRefs.current.get(id);
      const preferred =
        part === "name"
          ? refs?.nameLink
          : part === "rename"
            ? refs?.renameButton
            : refs?.menuButton;
      const target =
        preferred?.isConnected === true && (part === "name" || isShown(preferred))
          ? preferred
          : refs?.actionsButton;
      if (target?.isConnected === true) target.focus();
      else newKindRef.current?.focus();
    }, 0);
  }

  // Callers block starting/cancelling edits while pending through disabled actions and guarded row inputs.
  function openCreate() {
    setCreating(true);
    setCreateError(null);
    setRenamingId(null);
    setRenameError(null);
  }

  function cancelCreate() {
    setCreating(false);
    setCreateError(null);
    setTimeout(() => newKindRef.current?.focus(), 0);
  }

  async function submitCreate(name: string) {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createTagKind(name);
      setKinds((current) => withKind(current ?? [], created));
      setCreating(false);
      focusLater(created.id, "name");
    } catch (failure) {
      setCreateError(kindFieldError(failure));
    } finally {
      setCreatePending(false);
    }
  }

  function startRename(kind: TagKind) {
    setCreating(false);
    setCreateError(null);
    setRenameError(null);
    setRenamingId(kind.id);
  }

  function cancelRename(kind: TagKind) {
    setRenamingId(null);
    setRenameError(null);
    focusLater(kind.id, "rename");
  }

  function gone(order: readonly TagKind[], id: number) {
    changed.current = true;
    toast(t.tags.kinds.gone);
    focusLater(nextAfterRemoval(order, id)?.id, "rename");
  }

  async function submitRename(kind: TagKind, name: string) {
    setRenameError(null);
    setRenamePending(true);
    const order = kinds ?? [];
    try {
      const updated = await renameTagKind(kind.id, name);
      changed.current = true;
      setKinds((current) => withKind(current ?? [], updated));
      setRenamingId(null);
      focusLater(updated.id, "rename");
    } catch (failure) {
      if (isKindNotFound(failure)) {
        setRenamingId(null);
        gone(order, kind.id);
        return;
      }
      setRenameError(kindFieldError(failure));
    } finally {
      setRenamePending(false);
    }
  }

  function openDelete(kind: TagKind) {
    setDeleteError(null);
    setDeleting(kind);
  }

  function cancelDelete() {
    // ConfirmDialog still attempts Escape closure while pending; reject it here.
    if (deletePending || deleting === null) return;
    const target = deleting;
    setDeleting(null);
    setDeleteError(null);
    // The originating menu item unmounted; restore its row's menu trigger.
    focusLater(target.id, "menu");
  }

  async function performDelete(target: TagKind) {
    const order = kinds ?? [];
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteTagKind(target.id);
      changed.current = true;
      setKinds((current) => current?.filter((item) => item.id !== target.id));
      setDeleting(null);
      toast(t.tags.kinds.deleted(target.name));
      focusLater(nextAfterRemoval(order, target.id)?.id, "rename");
    } catch (failure) {
      if (isKindNotFound(failure)) {
        setDeleting(null);
        gone(order, target.id);
        return;
      }
      setDeleteError(errorText(failure));
    } finally {
      setDeletePending(false);
    }
  }

  function closeEdits() {
    setCreating(false);
    setCreateError(null);
    setRenamingId(null);
    setRenameError(null);
    setDeleting(null);
    setDeleteError(null);
  }

  function takeChanged(): boolean {
    const result = changed.current;
    changed.current = false;
    return result;
  }

  return {
    kinds,
    fresh,
    loadError,
    creating,
    createPending,
    createError,
    renamingId,
    renamePending,
    renameError,
    deleting,
    deletePending,
    deleteError,
    pending: createPending || renamePending || deletePending,
    editing: creating || renamingId !== null || deleting !== null,
    registerRefs,
    reload,
    openCreate,
    cancelCreate,
    submitCreate: (name) => void submitCreate(name),
    clearCreateError: () => setCreateError(null),
    startRename,
    cancelRename,
    submitRename: (kind, name) => void submitRename(kind, name),
    clearRenameError: () => setRenameError(null),
    openDelete,
    cancelDelete,
    performDelete: (kind) => void performDelete(kind),
    closeEdits,
    takeChanged,
  };
}
