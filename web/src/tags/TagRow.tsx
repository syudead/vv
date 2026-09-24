import { Ellipsis, Pencil, Trash2 } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { Tag } from "../api/tags";
import IconButton from "../ui/IconButton";
import { MenuContent, MenuItem, MenuRoot, MenuTrigger } from "../ui/Menu";
import { useTagNameField } from "./tagNameField";

export interface TagRowRefs {
  nameLink: HTMLAnchorElement | null;
  renameButton: HTMLButtonElement | null;
}

/**
 * TagRow は管理画面の一覧の1行である（ui-design.md「Rows」「Create and
 * rename」）。改名中は、名前の場所を同じ入力に置き換える。削除は行に直接
 * 出さず「その他の操作」のメニューへ置く（UI品質「削除と統合を、作成・改名
 * より目立たせない」）。統合とシノニムは Issue 272 で足す。
 */
export default function TagRow({
  tag,
  renaming,
  pending,
  error,
  registerRefs,
  onStartRename,
  onCancelRename,
  onSubmitRename,
  onDelete,
}: {
  tag: Tag;
  renaming: boolean;
  pending: boolean;
  error: string | null;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
  onStartRename: (tag: Tag) => void;
  onCancelRename: () => void;
  onSubmitRename: (tag: Tag, name: string) => void;
  onDelete: (tag: Tag) => void;
}) {
  const field = useTagNameField(tag.name);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!renaming) return;
    field.setValue(tag.name);
    inputRef.current?.focus();
    inputRef.current?.select();
    // 改名を開くたびに今の名前へ戻し、選んだ状態にする。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [renaming]);

  if (renaming) {
    const reasonId = `tag-rename-reason-${String(tag.id)}`;
    const errorId = `tag-rename-error-${String(tag.id)}`;
    return (
      <div data-tag-id={tag.id} className="flex flex-wrap items-center gap-2 py-2">
        <input
          ref={inputRef}
          value={field.value}
          onChange={(event) => field.setValue(event.target.value)}
          onPaste={field.onPaste}
          onBeforeInput={field.onBeforeInput}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              const spelling = field.trySpelling();
              if (spelling !== null) onSubmitRename(tag, spelling);
            } else if (event.key === "Escape") {
              event.preventDefault();
              onCancelRename();
            }
          }}
          aria-label={`「${tag.name}」の新しい名前`}
          aria-describedby={
            field.reason !== null ? reasonId : error !== null ? errorId : undefined
          }
          disabled={pending}
          className="h-8 min-w-0 flex-1 rounded-sm border border-border bg-field px-2 text-sm text-fg focus:border-accent focus:outline-none"
        />
        {field.reason !== null && (
          <p id={reasonId} className="w-full text-xs text-danger">
            {field.reason}
          </p>
        )}
        {field.reason === null && error !== null && (
          <p id={errorId} role="alert" className="w-full text-xs text-danger">
            {error}
          </p>
        )}
      </div>
    );
  }

  return (
    <div data-tag-id={tag.id} className="flex items-center gap-3 py-2">
      <div className="min-w-0 flex-1">
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
        >
          <Pencil />
        </IconButton>
        <MenuRoot>
          <MenuTrigger asChild>
            <IconButton label="その他の操作" size="sm">
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
