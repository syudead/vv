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

/** KindRowRefs は種類の行の、フォーカスの行き先になる要素である。 */
export interface KindRowRefs {
  nameLink: HTMLAnchorElement | null;
  renameButton: HTMLButtonElement | null;
  menuButton: HTMLButtonElement | null;
  /** タッチの端末と `sm` 未満で、行の操作をまとめた「Actions」の入口。 */
  actionsButton: HTMLButtonElement | null;
}

/** withKind は 1 件を足すか置き換え、名前の位置へ並べ直す。 */
export function withKind(kinds: readonly TagKind[], kind: TagKind): TagKind[] {
  return sortTagKinds([...kinds.filter((item) => item.id !== kind.id), kind]);
}

/** nextAfterRemoval は消えた行の次の行（無ければ前の行）を返す。 */
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

/** TagKindsTab はタグ管理画面の「Kinds」のタブが描く状態と操作である。 */
export interface TagKindsTab {
  /** 名前の自然順の種類。undefined の間は読み込み中か、読み込みの失敗。 */
  kinds: TagKind[] | undefined;
  /**
   * fresh は、画面を開いてから種類の一覧を 1 度でも読めたかである。前の画面の保持だけの
   * 間は false で、URL の `kind` がもう無いかはこれが true になってから決める
   * （specs/051-tag-kinds/ui-design.md「Kind filter」）。
   */
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
  /** 作成・改名・削除の送信中。タブを切り替えない。 */
  pending: boolean;
  /** 作成の行・改名の入力・削除の確認の窓のどれかが開いている。 */
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
  performDelete: () => void;
  /** タブを離れるときに、作成の行・改名の入力・削除の確認の窓を閉じる。 */
  closeEdits: () => void;
  /**
   * takeChanged は、前に呼んだときから種類の改名・削除（もう無かったときを含む）が
   * あったかを返し、印を消す。タブを離れるときに「Tags」のタブを読み直すかを決める。
   */
  takeChanged: () => boolean;
}

/**
 * useTagKindsTab はタグ管理画面の「Kinds」のタブの状態を持つ
 * （specs/051-tag-kinds/ui-design.md「Kinds tab」）。種類の一覧は `api/tagKinds.ts` の
 * 共有の保持を読み、取り直されるたびに追う。画面を開いたときは保持があっても取り直し
 * （ほかのタブで種類が変わったかもしれない。タグの削除・統合などで `tagCount` も変わる）、
 * タブの件数・「Tags」のタブの種類の絞り込みと「Set kind…」のメニューにも使う。
 * 作成・改名・削除の直後は読み込んだ並びをその場で書き換え、共有の保持の取り直しは
 * `api/tagKinds.ts` が始める。
 */
export function useTagKindsTab({
  toast,
  newKindRef,
}: {
  toast: ShowToast;
  /** 見出しの行の「New kind」。行へフォーカスを移せないときの行き先。 */
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

  /**
   * reload は種類の一覧をサーバーから取り直す。「Kinds」のタブを開いたときと Retry で
   * 呼ぶ（タグの削除・統合などで `tagCount` が変わっているかもしれない）。読み込んだ
   * 並びがあれば取り直しの間は並びを残す。失敗したら `loadError` を立て、パネルは古い
   * 数の並びではなく失敗と Retry を描く。
   */
  const reload = useCallback(() => {
    setLoadError(null);
    refreshTagKinds().catch((failure: unknown) => setLoadError(errorText(failure)));
  }, []);

  const registerRefs = useCallback((id: number, refs: Partial<KindRowRefs>) => {
    const current = rowRefs.current.get(id) ?? {};
    rowRefs.current.set(id, { ...current, ...refs });
  }, []);

  /**
   * focusLater は描き直しのあとで、その行の部分へフォーカスを移す。行の操作が「Actions」
   * 1 つにまとまっている（タッチの端末と `sm` 未満）ときは「Actions」へ。行が無ければ
   * 「New kind」へ。
   */
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

  function openCreate() {
    if (createPending || renamePending) return;
    setCreating(true);
    setCreateError(null);
    setRenamingId(null);
    setRenameError(null);
  }

  function cancelCreate() {
    if (createPending) return;
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
    if (createPending || renamePending) return;
    setCreating(false);
    setCreateError(null);
    setRenameError(null);
    setRenamingId(kind.id);
  }

  function cancelRename(kind: TagKind) {
    if (renamePending) return;
    setRenamingId(null);
    setRenameError(null);
    focusLater(kind.id, "rename");
  }

  /**
   * gone は改名・削除の `404 tag_kind_not_found` を扱う。入力や窓を閉じ、トーストを出す。
   * 一覧の取り直しは `api/tagKinds.ts` が始めている。フォーカスは消えた行の次の行へ。
   */
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
    if (deletePending || deleting === null) return;
    const target = deleting;
    setDeleting(null);
    setDeleteError(null);
    // 窓は「More actions」のメニューの項目から開いたので、その行の「More actions」へ戻す。
    focusLater(target.id, "menu");
  }

  async function performDelete() {
    if (deleting === null || deletePending) return;
    const target = deleting;
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
    performDelete: () => void performDelete(),
    closeEdits,
    takeChanged,
  };
}
