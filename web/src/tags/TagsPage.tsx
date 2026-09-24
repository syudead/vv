import { AlertCircle, Plus, SearchX, Tags as TagsIcon } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { errorMessage, RequestFailed } from "../api/client";
import { compareTagRefs } from "../api/tagOrder";
import {
  createTag,
  currentTags,
  deleteTag,
  refreshTags,
  renameTag,
  subscribeTags,
  type Tag,
} from "../api/tags";
import Button from "../ui/Button";
import Skeleton from "../ui/Skeleton";
import { useToast } from "../ui/Toast";
import { EmptyState } from "../videoList/states";
import CreateTagRow from "./CreateTagRow";
import DeleteTagDialog from "./DeleteTagDialog";
import type { TagFieldError } from "./tagNameField";
import TagRow, { type TagRowRefs } from "./TagRow";
import TagSearchBox from "./TagSearchBox";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function tagFieldError(failure: unknown): TagFieldError {
  if (failure instanceof RequestFailed && failure.code === "tag_name_taken") {
    return { kind: "taken", message: failure.message };
  }
  return { kind: "other", message: errorMessage(failure) };
}

type FocusTarget = "rename" | "name" | "menu";

/**
 * TagsPage はサイドバーの「タグ」から開く管理画面である（ui-design.md「Tag
 * management page」）。一覧・検索・作成・改名・削除を持つ。統合とシノニムは
 * Issue 272 で足す。
 *
 * タグの一覧は共有の保持（`web/src/api/tags.ts`、Plan の Structural
 * Decisions 8）を使う。作成・改名・削除の直後は、サーバーが返した最新の1件を
 * 今の一覧へその場で重ねる（もう1回 `GET /api/tags` を送らない。`createTag`・
 * `renameTag`・`deleteTag` 自体が共有の保持をバックグラウンドで取り直すので、
 * 二重の取得にはならない）。タグがもう無いとき（`tag_not_found`）だけ、
 * ほかのタグも変わっているかもしれないので `reload` で取り直す。
 */
export default function TagsPage() {
  const toast = useToast();
  const [tags, setTags] = useState<Tag[] | undefined>(currentTags());
  const [loadError, setLoadError] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);

  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);

  const [deletingTag, setDeletingTag] = useState<Tag | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
  const rowRefs = useRef(new Map<number, TagRowRefs>());

  const registerRefs = useCallback((id: number, refs: Partial<TagRowRefs>) => {
    const current = rowRefs.current.get(id) ?? {
      nameLink: null,
      renameButton: null,
      menuButton: null,
    };
    rowRefs.current.set(id, { ...current, ...refs });
  }, []);

  /**
   * focusRow はタグの行のフォーカス先へ移す。id が無ければ「新しいタグ」へ
   * 移す。「新しいタグ」は作成中に disabled になるので、`creating` を false に
   * 戻す更新が DOM に反映されたあとで移す必要がある（ほかの行への移動と同じく
   * setTimeout(0) で1呼吸置く。settings/SettingsPage.tsx の focusFolderAction
   * と同じ）。
   */
  const focusRow = useCallback((id: number | undefined, part: FocusTarget) => {
    if (id === undefined) {
      setTimeout(() => createButtonRef.current?.focus(), 0);
      return;
    }
    setTimeout(() => {
      const refs = rowRefs.current.get(id);
      const target =
        part === "rename"
          ? refs?.renameButton
          : part === "menu"
            ? refs?.menuButton
            : refs?.nameLink;
      if (target === null || target === undefined) createButtonRef.current?.focus();
      else target.focus();
    }, 0);
  }, []);

  /**
   * focusAfterRemoval は、行が一覧から消えた（削除、または tag_not_found の
   * 取り直しで消えた）あとのフォーカス先を決める。次の行の「改名」、無ければ
   * 前の行、1つも無ければ「新しいタグ」（ui-design.md「Merge and delete」）。
   * order は消える前の（絞り込み後の）並びである。
   */
  const focusAfterRemoval = useCallback(
    (order: readonly Tag[], removedId: number) => {
      const index = order.findIndex((tag) => tag.id === removedId);
      const remaining = order.filter((tag) => tag.id !== removedId);
      const next = remaining[Math.min(Math.max(index, 0), remaining.length - 1)];
      focusRow(next?.id, "rename");
    },
    [focusRow],
  );

  const reload = useCallback(() => {
    setLoadError(null);
    return refreshTags()
      .then((loaded) => {
        setTags(loaded);
        return loaded;
      })
      .catch((failure: unknown) => {
        // 既に一覧を持っているときは、その一覧を残したまま理由だけを控える
        // （読み込み失敗の空の状態は、一覧をまだ一度も取れていないときだけ
        // 出す。N6: 直前の操作は成功しているので、一覧を空白にしない）。
        setLoadError(errorMessage(failure));
        return undefined;
      });
  }, []);

  useEffect(() => {
    let alive = true;
    void reload();
    const unsubscribe = subscribeTags((loaded) => {
      if (alive) setTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, [reload]);

  // 一覧は `GET /api/tags` が返す名前の自然順（contracts/tags-api.md §3）を
  // 保つが、作成・改名でその場に重ねた1件は並びの外にあるかもしれないので
  // `compareTagRefs`（カード・再生画面・候補と同じ並び替え）で並べ直す。
  const sorted = useMemo(() => {
    if (tags === undefined) return [];
    return [...tags].sort(compareTagRefs);
  }, [tags]);

  const normalizedQuery = search.trim().toLowerCase();
  const filtered = useMemo(() => {
    if (normalizedQuery === "") return sorted;
    return sorted.filter(
      (tag) =>
        tag.name.toLowerCase().includes(normalizedQuery) ||
        tag.synonyms.some((synonym) => synonym.toLowerCase().includes(normalizedQuery)),
    );
  }, [sorted, normalizedQuery]);

  /**
   * visibleRows は実際に並べる行である。改名中の行は、検索を変えて一致しなく
   * なっても一覧から外さない（外すとその行が消え、打っている途中の名前を
   * 失う）。`filtered` に無ければ `sorted` から拾い、並び（`compareTagRefs`）を
   * 保つ位置へ差し込む。件数の行はこれを数えず、実際の一致件数（`filtered`）
   * のまま見せる。
   */
  const visibleRows = useMemo(() => {
    if (renamingId === null) return filtered;
    if (filtered.some((tag) => tag.id === renamingId)) return filtered;
    const renamingTag = sorted.find((tag) => tag.id === renamingId);
    if (renamingTag === undefined) return filtered;
    const insertAt = filtered.findIndex((tag) => compareTagRefs(tag, renamingTag) > 0);
    const at = insertAt === -1 ? filtered.length : insertAt;
    return [...filtered.slice(0, at), renamingTag, ...filtered.slice(at)];
  }, [filtered, sorted, renamingId]);

  const searching = normalizedQuery !== "";
  const total = tags?.length ?? 0;
  const countText =
    tags === undefined
      ? "読み込み中…"
      : searching
        ? `${String(filtered.length)} / ${String(total)} 個のタグ`
        : `${String(total)} 個のタグ`;

  function openCreate() {
    // 改名の送信中は、その応答が届くまで新しく作成を始めない（B2 と同じ規則。
    // 「新しいタグ」自体も createPending・creating では disabled だが、
    // renamePending はボタンの disabled 条件に含めているので、ここは主に
    // キーボード操作などボタンを介さない呼び出しへの保険である）。
    if (renamePending) return;
    setCreating(true);
    setCreateError(null);
    setRenamingId(null);
  }

  function clearSearch() {
    setSearch("");
    searchInputRef.current?.focus();
  }

  async function submitCreate(name: string) {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createTag(name);
      setTags((current) => (current === undefined ? [created] : [...current, created]));
      setCreating(false);
      focusRow(created.id, "name");
    } catch (failure) {
      setCreateError(tagFieldError(failure));
    } finally {
      setCreatePending(false);
    }
  }

  async function submitRename(tag: Tag, name: string) {
    setRenameError(null);
    setRenamePending(true);
    try {
      const updated = await renameTag(tag.id, name);
      setTags((current) =>
        current?.map((item) => (item.id === updated.id ? updated : item)),
      );
      setRenamingId(null);
      focusRow(tag.id, "rename");
    } catch (failure) {
      if (isTagNotFound(failure)) {
        const order = visibleRows;
        setRenamingId(null);
        toast("このタグはもう無いため、一覧を取り直しました");
        await reload();
        focusAfterRemoval(order, tag.id);
        return;
      }
      setRenameError(tagFieldError(failure));
    } finally {
      setRenamePending(false);
    }
  }

  async function performDelete() {
    if (deletingTag === null) return;
    const target = deletingTag;
    const order = visibleRows;
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteTag(target.id);
      setTags((current) => current?.filter((item) => item.id !== target.id));
      setDeletingTag(null);
      toast("削除しました");
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setDeletingTag(null);
        toast("このタグはもう無いため、一覧を取り直しました");
        await reload();
        focusAfterRemoval(order, target.id);
        return;
      }
      setDeleteError(errorMessage(failure));
    } finally {
      setDeletePending(false);
    }
  }

  function cancelDelete() {
    if (deletePending || deletingTag === null) return;
    const target = deletingTag;
    setDeletingTag(null);
    setDeleteError(null);
    // 窓は「その他の操作」のメニューの項目から開いた。その項目はメニューが
    // 閉じるともう無いので、ModalFrame の「前のフォーカスへ戻す」には頼らず、
    // その行の「その他の操作」へ明示的に戻す（B2）。
    focusRow(target.id, "menu");
  }

  const showEmptyTags = tags !== undefined && tags.length === 0 && !creating;
  const showNoMatch =
    tags !== undefined && tags.length > 0 && visibleRows.length === 0 && !creating;
  const showRows =
    tags !== undefined && (visibleRows.length > 0 || creating) && !showEmptyTags;

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-6 sm:py-8">
      <h1 className="text-xl font-semibold">タグ</h1>
      {/*
        h1・操作の行・件数の行の間隔は ui-design.md が固定していない（固定するのは
        本文の外側の余白と行の py-2 だけ）。「1280×800 でシノニムの行を持つタグと
        持たないタグが半々のとき、12 行以上が1画面に見える」（ui-design.md「Visual
        review criteria」情報密度）を満たすため、ここを詰める（B5）。
      */}

      <div className="mt-2 flex items-center gap-3">
        <TagSearchBox
          value={search}
          onChange={setSearch}
          inputRef={searchInputRef}
          disabled={tags !== undefined && tags.length === 0}
          className="flex-1 sm:max-w-sm"
        />
        <Button
          ref={createButtonRef}
          variant="secondary"
          onClick={openCreate}
          disabled={tags === undefined || creating || createPending || renamePending}
        >
          <Plus />
          新しいタグ
        </Button>
      </div>

      <p
        role="status"
        aria-live="polite"
        className="mt-1 text-xs text-fg-muted tabular-nums"
      >
        {countText}
      </p>

      <div className="mt-1">
        {tags === undefined && loadError === null && (
          <div className="space-y-2" aria-hidden="true">
            {Array.from({ length: 6 }, (_, index) => (
              <Skeleton key={index} className="h-10" />
            ))}
          </div>
        )}

        {tags === undefined && loadError !== null && (
          <EmptyState
            icon={AlertCircle}
            tone="danger"
            title="タグを取得できません"
            action={<Button onClick={() => void reload()}>再試行</Button>}
          />
        )}

        {showEmptyTags && (
          <EmptyState
            icon={TagsIcon}
            title="タグはまだありません"
            description="動画の再生画面や、ライブラリの選択バーから付けられます。ここで先に作っておくこともできます。"
            action={
              <Button variant="primary" onClick={openCreate}>
                <Plus />
                新しいタグ
              </Button>
            }
          />
        )}

        {showNoMatch && (
          <EmptyState
            icon={SearchX}
            title={`「${search}」に一致するタグはありません`}
            action={<Button onClick={clearSearch}>検索をクリア</Button>}
          />
        )}

        {showRows && (
          <div className="divide-y divide-border">
            {creating && (
              <CreateTagRow
                pending={createPending}
                error={createError}
                onCancel={() => {
                  // 送信中は、その応答が届くまで閉じない（B2 と同じ規則。
                  // CreateTagRow 自身の Esc・「キャンセル」も pending を見るが、
                  // ここでも二重に守る）。
                  if (createPending) return;
                  setCreating(false);
                  setCreateError(null);
                  focusRow(undefined, "name");
                }}
                onSubmit={(name) => void submitCreate(name)}
                onDraftChange={() => setCreateError(null)}
              />
            )}
            {visibleRows.map((tag) => (
              <TagRow
                key={tag.id}
                tag={tag}
                renaming={renamingId === tag.id}
                pending={renamingId === tag.id && renamePending}
                blockStart={createPending || (renamePending && renamingId !== tag.id)}
                error={renamingId === tag.id ? renameError : null}
                registerRefs={registerRefs}
                onStartRename={(target) => {
                  // ほかの行の改名や作成が送信中は、新しく改名を始めない
                  // （B2 と同じ規則）。行の「改名」自体も blockStart で
                  // disabled だが、ここでも二重に守る。
                  if (createPending || renamePending) return;
                  setCreating(false);
                  setRenameError(null);
                  setRenamingId(target.id);
                }}
                onCancelRename={() => {
                  if (renamePending) return;
                  setRenamingId(null);
                  setRenameError(null);
                  focusRow(tag.id, "rename");
                }}
                onSubmitRename={(target, name) => void submitRename(target, name)}
                onDelete={(target) => {
                  setDeleteError(null);
                  setDeletingTag(target);
                }}
                onDraftChange={() => setRenameError(null)}
              />
            ))}
          </div>
        )}
      </div>

      {deletingTag !== null && (
        <DeleteTagDialog
          tag={deletingTag}
          pending={deletePending}
          error={deleteError}
          onClose={cancelDelete}
          onDelete={() => void performDelete()}
        />
      )}
    </div>
  );
}
