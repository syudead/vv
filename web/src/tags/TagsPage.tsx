import { AlertCircle, Plus, SearchX, Tags as TagsIcon } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { errorMessage, RequestFailed } from "../api/client";
import { compareNatural } from "../api/tagOrder";
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
import TagRow, { type TagRowRefs } from "./TagRow";
import TagSearchBox from "./TagSearchBox";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

/**
 * TagsPage はサイドバーの「タグ」から開く管理画面である（ui-design.md「Tag
 * management page」）。一覧・検索・作成・改名・削除を持つ。統合とシノニムは
 * Issue 272 で足す。
 *
 * タグの一覧は共有の保持（`web/src/api/tags.ts`、Plan の Structural
 * Decisions 8）を使う。この画面が開くときは必ず取り直し、作成・改名・削除の
 * あとも取り直した結果でフォーカス先を決める（サーバーが返した最新の並びと
 * 本数を、取り直しを待たずに使い違えないため）。
 */
export default function TagsPage() {
  const toast = useToast();
  const [tags, setTags] = useState<Tag[] | undefined>(currentTags());
  const [loadError, setLoadError] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<string | null>(null);

  const [deletingTag, setDeletingTag] = useState<Tag | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
  const rowRefs = useRef(new Map<number, TagRowRefs>());

  const registerRefs = useCallback((id: number, refs: Partial<TagRowRefs>) => {
    const current = rowRefs.current.get(id) ?? { nameLink: null, renameButton: null };
    rowRefs.current.set(id, { ...current, ...refs });
  }, []);

  /** focusRow はタグの行のフォーカス先へ移す。id が無ければ「新しいタグ」へ移す。 */
  const focusRow = useCallback((id: number | undefined, part: "rename" | "name") => {
    if (id === undefined) {
      createButtonRef.current?.focus();
      return;
    }
    // DOM が更新されてから移す（settings/SettingsPage.tsx の focusFolderAction と同じ）。
    setTimeout(() => {
      const refs = rowRefs.current.get(id);
      const target = part === "rename" ? refs?.renameButton : refs?.nameLink;
      if (target === null || target === undefined) createButtonRef.current?.focus();
      else target.focus();
    }, 0);
  }, []);

  const reload = useCallback(() => {
    setLoadError(null);
    return refreshTags()
      .then((loaded) => {
        setTags(loaded);
        return loaded;
      })
      .catch((failure: unknown) => {
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

  const sorted = useMemo(() => {
    if (tags === undefined) return [];
    return [...tags].sort((a, b) => compareNatural(a.name, b.name) || a.id - b.id);
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

  const searching = search !== "";
  const total = tags?.length ?? 0;
  const countText =
    tags === undefined
      ? "読み込み中…"
      : searching
        ? `${String(filtered.length)} / ${String(total)} 個のタグ`
        : `${String(total)} 個のタグ`;

  function openCreate() {
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
      await reload();
      setCreating(false);
      focusRow(created.id, "name");
    } catch (failure) {
      if (failure instanceof RequestFailed && failure.code === "tag_name_taken") {
        setCreateError(failure.message);
      } else {
        setCreateError(errorMessage(failure));
      }
    } finally {
      setCreatePending(false);
    }
  }

  async function submitRename(tag: Tag, name: string) {
    setRenameError(null);
    setRenamePending(true);
    try {
      await renameTag(tag.id, name);
      await reload();
      setRenamingId(null);
      focusRow(tag.id, "rename");
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRenamingId(null);
        toast("このタグはもう無いため、一覧を取り直しました");
        void reload();
        return;
      }
      if (failure instanceof RequestFailed && failure.code === "tag_name_taken") {
        setRenameError(failure.message);
      } else {
        setRenameError(errorMessage(failure));
      }
    } finally {
      setRenamePending(false);
    }
  }

  async function performDelete() {
    if (deletingTag === null) return;
    const target = deletingTag;
    const order = filtered;
    const index = order.findIndex((tag) => tag.id === target.id);
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteTag(target.id);
      await reload();
      setDeletingTag(null);
      toast("削除しました");
      const remaining = order.filter((tag) => tag.id !== target.id);
      const next = remaining[Math.min(index, remaining.length - 1)];
      focusRow(next?.id, "rename");
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setDeletingTag(null);
        toast("このタグはもう無いため、一覧を取り直しました");
        void reload();
        return;
      }
      setDeleteError(errorMessage(failure));
    } finally {
      setDeletePending(false);
    }
  }

  const showEmptyTags = tags !== undefined && tags.length === 0 && !creating;
  const showNoMatch =
    tags !== undefined && tags.length > 0 && filtered.length === 0 && !creating;
  const showRows =
    tags !== undefined && (filtered.length > 0 || creating) && !showEmptyTags;

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-6 sm:py-8">
      <h1 className="text-xl font-semibold">タグ</h1>

      <div className="mt-6 flex items-center gap-3">
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
          disabled={tags === undefined || creating || createPending}
        >
          <Plus />
          新しいタグ
        </Button>
      </div>

      <p
        role="status"
        aria-live="polite"
        className="mt-3 text-xs text-fg-muted tabular-nums"
      >
        {countText}
      </p>

      <div className="mt-2">
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
                  setCreating(false);
                  setCreateError(null);
                }}
                onSubmit={(name) => void submitCreate(name)}
              />
            )}
            {filtered.map((tag) => (
              <TagRow
                key={tag.id}
                tag={tag}
                renaming={renamingId === tag.id}
                pending={renamingId === tag.id && renamePending}
                error={renamingId === tag.id ? renameError : null}
                registerRefs={registerRefs}
                onStartRename={(target) => {
                  setCreating(false);
                  setRenameError(null);
                  setRenamingId(target.id);
                }}
                onCancelRename={() => {
                  setRenamingId(null);
                  setRenameError(null);
                }}
                onSubmitRename={(target, name) => void submitRename(target, name)}
                onDelete={(target) => {
                  setDeleteError(null);
                  setDeletingTag(target);
                }}
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
          onClose={() => {
            if (!deletePending) setDeletingTag(null);
          }}
          onDelete={() => void performDelete()}
        />
      )}
    </div>
  );
}
