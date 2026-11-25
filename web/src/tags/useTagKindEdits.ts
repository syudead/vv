import { type RefObject, useCallback, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  createTagKind,
  deleteTagKind,
  renameTagKind,
  setTagKindCardFirst,
  sortTagKinds,
  type TagKind,
} from "../api/tagKinds";
import { errorText, t, type UiText } from "../i18n";
import type { ShowToast } from "../ui/Toast";
import type { TagFieldError } from "../ui/nameField";
import { kindFieldError } from "./tagNameField";
import { useTagKinds } from "./useTagKinds";

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

export interface TagKindEdits {
  kinds: TagKind[] | undefined;
  loadError: UiText | null;
  createPending: boolean;
  createError: TagFieldError | null;
  renamingId: number | null;
  renamePending: boolean;
  renameError: TagFieldError | null;
  deleting: TagKind | null;
  deletePending: boolean;
  deleteError: UiText | null;
  /** The kind whose card order is being saved. */
  cardFirstPendingId: number | null;
  registerMenu: (id: number, menu: HTMLButtonElement | null) => void;
  reload: () => void;
  /** Resolves true when the kind was created, so the field can clear itself. */
  submitCreate: (name: string) => Promise<boolean>;
  clearCreateError: () => void;
  startRename: (kind: TagKind) => void;
  cancelRename: (kind: TagKind) => void;
  submitRename: (kind: TagKind, name: string) => void;
  clearRenameError: () => void;
  openDelete: (kind: TagKind) => void;
  cancelDelete: () => void;
  performDelete: (kind: TagKind) => void;
  setCardFirst: (kind: TagKind, cardFirst: boolean) => void;
}

/** Apply edits locally while the API refreshes the shared kinds; focus returns to a row's menu or the New kind field. */
export function useTagKindEdits({
  toast,
  newKindRef,
}: {
  toast: ShowToast;
  newKindRef: RefObject<HTMLInputElement | null>;
}): TagKindEdits {
  const { kinds, loadError, setKinds, reload } = useTagKinds();
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);
  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);
  const [deleting, setDeleting] = useState<TagKind | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);
  const [cardFirstPendingId, setCardFirstPendingId] = useState<number | null>(null);
  const menus = useRef(new Map<number, HTMLButtonElement>());

  const registerMenu = useCallback((id: number, menu: HTMLButtonElement | null) => {
    if (menu === null) menus.current.delete(id);
    else menus.current.set(id, menu);
  }, []);

  /** Wait for the row to render or leave before choosing the target. */
  function focusLater(id: number | undefined) {
    setTimeout(() => {
      const menu = id === undefined ? undefined : menus.current.get(id);
      if (menu?.isConnected === true) menu.focus();
      else newKindRef.current?.focus();
    }, 0);
  }

  async function submitCreate(name: string): Promise<boolean> {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createTagKind(name);
      setKinds((current) => withKind(current ?? [], created));
      return true;
    } catch (failure) {
      setCreateError(kindFieldError(failure));
      return false;
    } finally {
      setCreatePending(false);
    }
  }

  function startRename(kind: TagKind) {
    setRenameError(null);
    setRenamingId(kind.id);
  }

  function cancelRename(kind: TagKind) {
    setRenamingId(null);
    setRenameError(null);
    focusLater(kind.id);
  }

  function gone(order: readonly TagKind[], id: number) {
    toast(t.tags.kinds.gone);
    focusLater(nextAfterRemoval(order, id)?.id);
  }

  async function submitRename(kind: TagKind, name: string) {
    setRenameError(null);
    setRenamePending(true);
    const order = kinds ?? [];
    try {
      const updated = await renameTagKind(kind.id, name);
      setKinds((current) => withKind(current ?? [], updated));
      setRenamingId(null);
      focusLater(updated.id);
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
    focusLater(target.id);
  }

  async function performDelete(target: TagKind) {
    const order = kinds ?? [];
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteTagKind(target.id);
      setKinds((current) => current?.filter((item) => item.id !== target.id));
      setDeleting(null);
      toast(t.tags.kinds.deleted(target.name));
      focusLater(nextAfterRemoval(order, target.id)?.id);
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

  async function setCardFirst(kind: TagKind, cardFirst: boolean) {
    const order = kinds ?? [];
    setCardFirstPendingId(kind.id);
    try {
      const updated = await setTagKindCardFirst(kind.id, cardFirst);
      setKinds((current) => withKind(current ?? [], updated));
    } catch (failure) {
      if (isKindNotFound(failure)) {
        gone(order, kind.id);
        return;
      }
      toast(errorText(failure));
    } finally {
      setCardFirstPendingId(null);
    }
  }

  return {
    kinds,
    loadError,
    createPending,
    createError,
    renamingId,
    renamePending,
    renameError,
    deleting,
    deletePending,
    deleteError,
    cardFirstPendingId,
    registerMenu,
    reload,
    submitCreate,
    clearCreateError: () => setCreateError(null),
    startRename,
    cancelRename,
    submitRename: (kind, name) => void submitRename(kind, name),
    clearRenameError: () => setRenameError(null),
    openDelete,
    cancelDelete,
    performDelete: (kind) => void performDelete(kind),
    setCardFirst: (kind, cardFirst) => void setCardFirst(kind, cardFirst),
  };
}
