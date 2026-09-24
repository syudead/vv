import { Ellipsis, Pencil, Trash2 } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { Tag } from "../api/tags";
import { isComposingKeyEvent } from "../ui/Combobox";
import IconButton from "../ui/IconButton";
import { MenuContent, MenuItem, MenuRoot, MenuTrigger } from "../ui/Menu";
import { useTagNameField, type TagFieldError } from "./tagNameField";

export interface TagRowRefs {
  nameLink: HTMLAnchorElement | null;
  renameButton: HTMLButtonElement | null;
  menuButton: HTMLButtonElement | null;
}

/**
 * TagRow は管理画面の一覧の1行である（ui-design.md「Rows」「Create and
 * rename」）。改名中は、名前の場所（列）だけを同じ入力に置き換える。本数・
 * 「改名」・「その他の操作」は出したままにし、行の高さも変えない
 * （行全体を作成の行のような別レイアウトに差し替えない）。削除は行に直接
 * 出さず「その他の操作」のメニューへ置く（UI品質「削除と統合を、作成・改名
 * より目立たせない」）。統合とシノニムは Issue 272 で足す。
 */
export default function TagRow({
  tag,
  renaming,
  pending,
  blockStart,
  error,
  registerRefs,
  onStartRename,
  onCancelRename,
  onSubmitRename,
  onDelete,
  onDraftChange,
}: {
  tag: Tag;
  renaming: boolean;
  pending: boolean;
  /** ほかの行の作成・改名が送信中で、この行では新しく改名や削除を始められない。 */
  blockStart: boolean;
  error: TagFieldError | null;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
  onStartRename: (tag: Tag) => void;
  onCancelRename: () => void;
  onSubmitRename: (tag: Tag, name: string) => void;
  onDelete: (tag: Tag) => void;
  /** 値が変わるたびに呼ぶ。呼び出し元はこれで直前の失敗の表示を消す。 */
  onDraftChange?: () => void;
}) {
  const field = useTagNameField(tag.name, onDraftChange);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!renaming) return;
    field.setValue(tag.name);
    inputRef.current?.focus();
    inputRef.current?.select();
    // 改名を開くたびに今の名前へ戻し、選んだ状態にする。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [renaming]);

  function submit() {
    if (pending) return;
    const spelling = field.trySpelling();
    if (spelling !== null) onSubmitRename(tag, spelling);
  }

  function cancel() {
    // 送信中は、その応答が届くまで閉じない（Esc の場合。この行に「キャンセル」
    // のボタンは無い。ui-design.md「Create and rename」）。閉じたあとに届いた
    // 応答が、もう無いこの行や別の行の状態を書き換えることを防ぐ。
    if (pending) return;
    onCancelRename();
  }

  const reasonId = `tag-rename-reason-${String(tag.id)}`;
  const errorId = `tag-rename-error-${String(tag.id)}`;

  return (
    <div data-tag-id={tag.id} className="flex items-center gap-3 py-2">
      <div className="min-w-0 flex-1">
        {renaming ? (
          <input
            ref={inputRef}
            value={field.value}
            onChange={(event) => field.setValue(event.target.value)}
            onPaste={field.onPaste}
            onBeforeInput={field.onBeforeInput}
            onKeyDown={(event) => {
              if (isComposingKeyEvent(event)) return;
              if (event.key === "Enter") {
                event.preventDefault();
                submit();
              } else if (event.key === "Escape") {
                event.preventDefault();
                cancel();
              }
            }}
            aria-label={`「${tag.name}」の新しい名前`}
            aria-describedby={
              field.reason !== null ? reasonId : error !== null ? errorId : undefined
            }
            aria-busy={pending || undefined}
            className="h-8 w-full min-w-0 rounded-sm border border-border bg-field px-2 text-sm text-fg focus:border-accent focus:outline-none"
          />
        ) : (
          <>
            <Link
              ref={(node) => registerRefs(tag.id, { nameLink: node })}
              to={`/?tag=${String(tag.id)}`}
              title={tag.name}
              aria-label={`${tag.name}で絞り込んだライブラリを開く`}
              className="block truncate text-sm font-medium text-fg hover:text-link"
            >
              {tag.name}
            </Link>
            {tag.synonyms.length > 0 && (
              <p
                title={`シノニム: ${tag.synonyms.join(" · ")}`}
                className="truncate text-xs text-fg-muted"
              >
                シノニム: {tag.synonyms.join(" · ")}
              </p>
            )}
          </>
        )}
        {renaming && field.reason !== null && (
          <p id={reasonId} className="mt-1 text-xs text-danger">
            {field.reason}
          </p>
        )}
        {renaming &&
          field.reason === null &&
          error !== null &&
          error.kind === "taken" && (
            <p id={errorId} className="mt-1 text-xs text-danger">
              {error.message}
            </p>
          )}
        {renaming &&
          field.reason === null &&
          error !== null &&
          error.kind === "other" && (
            <p id={errorId} role="alert" className="mt-1 text-sm text-danger">
              {error.message}
            </p>
          )}
      </div>
      <span className="w-16 shrink-0 text-right text-sm text-fg-muted tabular-nums">
        {tag.videoCount} 本
      </span>
      <div className="flex shrink-0 items-center gap-1">
        <IconButton
          ref={(node) => registerRefs(tag.id, { renameButton: node })}
          label="改名"
          size="sm"
          onClick={() => onStartRename(tag)}
          disabled={renaming || blockStart}
        >
          <Pencil />
        </IconButton>
        <MenuRoot>
          <MenuTrigger asChild>
            <IconButton
              ref={(node) => registerRefs(tag.id, { menuButton: node })}
              label="その他の操作"
              size="sm"
              disabled={renaming || blockStart}
            >
              <Ellipsis />
            </IconButton>
          </MenuTrigger>
          <MenuContent>
            <MenuItem tone="danger" onSelect={() => onDelete(tag)}>
              <Trash2 />
              削除…
            </MenuItem>
          </MenuContent>
        </MenuRoot>
      </div>
    </div>
  );
}
