import {
  Ban,
  Check,
  Ellipsis,
  LoaderCircle,
  Merge,
  Pencil,
  Tags as SynonymsIcon,
  Trash2,
} from "lucide-react";
import { memo, useEffect, useEffectEvent, useRef } from "react";
import { Link } from "react-router";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import Checkbox from "../ui/legacy/Checkbox";
import { isComposingKeyEvent } from "../ui/legacy/Combobox";
import IconButton from "../ui/legacy/IconButton";
import { MenuContent, MenuItem, MenuRoot, MenuSeparator, MenuTrigger } from "../ui/Menu";
import TentativeMark from "../ui/TentativeMark";
import Tooltip from "../ui/Tooltip";
import { useTagNameField, type TagFieldError } from "./tagNameField";

export interface TagRowRefs {
  nameLink: HTMLAnchorElement | null;
  renameButton: HTMLButtonElement | null;
  synonymsButton: HTMLButtonElement | null;
  menuButton: HTMLButtonElement | null;
  /** タッチの端末と `sm` 未満で、行の操作をまとめた「Actions」の入口。 */
  actionsButton: HTMLButtonElement | null;
}

/**
 * TagRow は管理画面の一覧の1行である（ui-design.md「Rows」「Create and
 * rename」）。改名中は、名前の場所（列）だけを同じ入力に置き換える。本数・
 * 「改名」・「その他の操作」は出したままにし、行の高さも変えない
 * （行全体を作成の行のような別レイアウトに差し替えない）。削除は行に直接
 * 出さず「その他の操作」のメニューへ置く（UI品質「削除と統合を、作成・改名
 * より目立たせない」）。「シノニム」は改名の右に直接置き、「別のタグへ
 * 統合…」は「その他の操作」の先頭・区切り線の上に置く（ui-design.md「Rows」）。
 *
 * 仮のタグ（`tentative`）の行は、名前の後ろに仮の目印を付け、操作の一群の
 * 先頭に「確定する」を置き、メニューの「削除…」を「却下する…」に置き換える
 * （specs/031-tentative-tags/ui-design.md「Row」）。行の高さ・本数の列・改名の
 * 入力は確定した行と同じ。
 *
 * 名前の列の左に行のチェックを置き、選んだ行は面を `primary-soft` にする。
 * タッチの端末（`pointer: coarse`）と `sm` 未満の幅では、行の操作を文字を持つ
 * 1 つのメニュー「Actions」にまとめる。出し分けは CSS だけで行う
 * （specs/036-tag-admin-scale/ui-design.md「Row checkbox」「Actions on touch and
 * narrow widths」）。
 *
 * 一覧は見えている行だけを描き、スクロールや検索のたびに行を描き直すので、
 * `React.memo` で包む（specs/036-tag-admin-scale/research.md R-2）。呼び出し元は
 * 行の props（関数を含む）を安定させる。
 */
export default memo(TagRow);

function TagRow({
  tag,
  renaming,
  pending,
  blockStart,
  error,
  registerRefs,
  onStartRename,
  onCancelRename,
  onSubmitRename,
  onOpenSynonyms,
  onOpenMerge,
  onDelete,
  confirming = false,
  onConfirm,
  onReject,
  onDraftChange,
  selected = false,
  selectionActive = false,
  onSelect,
}: {
  tag: Tag;
  renaming: boolean;
  pending: boolean;
  /** ほかの行の作成・改名が送信中で、この行では新しく改名や削除を始められない。 */
  blockStart: boolean;
  error: TagFieldError | null;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
  onStartRename: (tag: Tag) => void;
  onCancelRename: (tag: Tag) => void;
  onSubmitRename: (tag: Tag, name: string) => void;
  onOpenSynonyms: (tag: Tag) => void;
  onOpenMerge: (tag: Tag) => void;
  onDelete: (tag: Tag) => void;
  /** 仮のタグの確定の要求が届いている間 true。「確定する」は次の押下を無視する。 */
  confirming?: boolean;
  onConfirm?: (tag: Tag) => void;
  onReject?: (tag: Tag) => void;
  /** 値が変わるたびに呼ぶ。呼び出し元はこれで直前の失敗の表示を消す。 */
  onDraftChange?: () => void;
  /** この行を選んでいる。 */
  selected?: boolean;
  /** どれか 1 行でも選んでいる。その間はすべての行のチェックを薄くしない。 */
  selectionActive?: boolean;
  onSelect?: (tag: Tag, selected: boolean) => void;
}) {
  const field = useTagNameField(tag.name, onDraftChange);
  const inputRef = useRef<HTMLInputElement | null>(null);

  // 改名を開くたびに今の名前へ戻し、選んだ状態にする。きっかけは renaming だけで、
  // 開いている間に名前や field が変わっても入力中の値は戻さない。
  const startRenaming = useEffectEvent(() => {
    field.setValue(tag.name);
    inputRef.current?.focus();
    inputRef.current?.select();
  });
  useEffect(() => {
    if (renaming) startRenaming();
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
    onCancelRename(tag);
  }

  const reasonId = `tag-rename-reason-${String(tag.id)}`;
  const errorId = `tag-rename-error-${String(tag.id)}`;

  return (
    <div
      data-tag-id={tag.id}
      className={cn(
        "group flex items-center gap-2 rounded-md px-2 py-2 sm:gap-3",
        renaming
          ? "bg-elevated ring-1 ring-control-border"
          : // 選んだ行はアクセントの薄い色（10%）で塗る。
            selected && "bg-primary/10",
      )}
    >
      {/*
        押す範囲はチェックを中央に置いた size-8 の正方形（タッチでも外さない）。
        包みの余白を押しても同じチェックを切り替える。改名中の行は選ばない
        （改名の確定で並びの別の位置へ動きうるため。ui-design.md「Row checkbox」）。
      */}
      <div
        className="-my-1.5 flex size-8 shrink-0 items-center justify-center"
        onClick={(event) => {
          if (event.target !== event.currentTarget || renaming) return;
          onSelect?.(tag, !selected);
        }}
      >
        <Checkbox
          checked={selected && !renaming}
          onCheckedChange={(next) => onSelect?.(tag, next)}
          label={t.tags.row.select(tag.name)}
          disabled={renaming}
          className={cn(
            "transition-opacity",
            selectionActive || selected
              ? "opacity-100"
              : "opacity-40 group-focus-within:opacity-100 group-hover:opacity-100",
          )}
        />
      </div>
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
            aria-label={t.tags.row.renameLabel(tag.name)}
            aria-describedby={
              field.reason !== null ? reasonId : error !== null ? errorId : undefined
            }
            aria-busy={pending || undefined}
            aria-invalid={field.reason !== null || error?.kind === "taken" || undefined}
            className="h-8 w-full min-w-0 rounded-sm border border-control-border bg-field px-2 text-sm text-fg focus:border-primary focus:outline-none focus:ring-2 focus:ring-link"
          />
        ) : (
          <>
            {/*
              名前は省略されても、仮の目印は shrink-0 で名前の後ろに残す
              （ui-design.md「Tentative mark」）。行を探す試験が名前の列の div を
              `closest("div")` で辿るので、ここの包みは span にする。
              Link は文字の幅に縮むので、`after:` の疑似要素を包み（relative）
              いっぱいに広げ、名前の列の空白を押しても絞り込んだ一覧が開くように
              する。目印は relative で疑似要素の上に置き、Tooltip を受けられる
              ようにする。
            */}
            <span className="relative flex min-w-0 items-center gap-1">
              <Link
                ref={(node) => registerRefs(tag.id, { nameLink: node })}
                to={`/?tag=${String(tag.id)}`}
                title={tag.name}
                aria-label={t.tags.row.open(tag.name)}
                className="block min-w-0 truncate text-sm font-medium text-fg after:absolute after:inset-0 hover:text-link"
              >
                {tag.name}
              </Link>
              {tag.tentative && (
                <Tooltip content={t.tags.tentative}>
                  <span className="relative inline-flex shrink-0">
                    <TentativeMark size="row" />
                    <span className="sr-only">{t.tags.tentative}</span>
                  </span>
                </Tooltip>
              )}
            </span>
            {tag.synonyms.length > 0 && (
              <p
                title={t.tags.row.synonyms(tag.synonyms.join(" · "))}
                className="truncate text-xs text-fg-muted"
              >
                {t.tags.row.synonyms(tag.synonyms.join(" · "))}
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
      <span className="w-16 shrink-0 whitespace-nowrap text-right text-xs text-fg-muted tabular-nums sm:w-20 sm:text-sm">
        {t.tags.row.videoCount(tag.videoCount)}
      </span>
      <CompactActions
        tag={tag}
        renaming={renaming}
        blockStart={blockStart}
        confirming={confirming}
        registerRefs={registerRefs}
        onStartRename={onStartRename}
        onOpenSynonyms={onOpenSynonyms}
        onOpenMerge={onOpenMerge}
        onDelete={onDelete}
        onConfirm={onConfirm}
        onReject={onReject}
      />
      <div className="flex shrink-0 items-center gap-1 max-sm:hidden [@media(pointer:coarse)]:hidden">
        {/*
          確定したタグの行も「確定する」の分の幅を空けておき、本数の列を列の見出しの
          「Videos」と全部の行でそろえる（specs/036-tag-admin-scale/ui-design.md
          「Column header」）。
        */}
        {!tag.tentative && <span aria-hidden="true" className="size-8 shrink-0" />}
        {tag.tentative && (
          <IconButton
            label={t.tags.row.confirm}
            size="sm"
            aria-busy={confirming || undefined}
            aria-disabled={confirming || undefined}
            onClick={() => {
              // 送信中は disabled にせず（押した本人のボタンのフォーカスを
              // 保つため）、次の押下を無視して要求を1回だけにする（ui-design.md
              // 「Confirm」）。
              if (confirming) return;
              onConfirm?.(tag);
            }}
            disabled={renaming}
          >
            {confirming ? (
              <LoaderCircle className="animate-spin motion-reduce:animate-none" />
            ) : (
              <Check />
            )}
          </IconButton>
        )}
        <IconButton
          ref={(node) => registerRefs(tag.id, { renameButton: node })}
          label={t.tags.row.rename}
          size="sm"
          onClick={() => onStartRename(tag)}
          disabled={renaming || blockStart || confirming}
        >
          <Pencil />
        </IconButton>
        <IconButton
          ref={(node) => registerRefs(tag.id, { synonymsButton: node })}
          label={t.tags.row.synonymsButton}
          size="sm"
          onClick={() => onOpenSynonyms(tag)}
          disabled={renaming}
        >
          <SynonymsIcon />
        </IconButton>
        <MenuRoot>
          <MenuTrigger asChild>
            <IconButton
              ref={(node) => registerRefs(tag.id, { menuButton: node })}
              label={t.tags.row.more}
              size="sm"
              disabled={renaming || blockStart || confirming}
            >
              <Ellipsis />
            </IconButton>
          </MenuTrigger>
          <MenuContent>
            <MenuItem onSelect={() => onOpenMerge(tag)}>
              <Merge />
              {t.tags.row.merge}
            </MenuItem>
            <MenuSeparator />
            {tag.tentative ? (
              <MenuItem tone="danger" onSelect={() => onReject?.(tag)}>
                <Ban />
                {t.tags.row.reject}
              </MenuItem>
            ) : (
              <MenuItem tone="danger" onSelect={() => onDelete(tag)}>
                <Trash2 />
                {t.tags.row.delete}
              </MenuItem>
            )}
          </MenuContent>
        </MenuRoot>
      </div>
    </div>
  );
}

/**
 * CompactActions は、タッチの端末と `sm` 未満の幅で行の右端に出す「Actions」の
 * メニューである。項目は文字を持ち、行の `IconButton` と「その他の操作」の項目と
 * 同じ振る舞いをする（ui-design.md「Actions on touch and narrow widths」）。
 * マウスの端末で `sm` 以上では CSS で隠す（`matchMedia` は読まない）。
 *
 * DOM では行の `IconButton` の一群より前に置く。どちらか一方だけが描かれるので
 * 見た目の順は変わらず、隠れている一群は Tab の順に入らない。
 */
function CompactActions({
  tag,
  renaming,
  blockStart,
  confirming,
  registerRefs,
  onStartRename,
  onOpenSynonyms,
  onOpenMerge,
  onDelete,
  onConfirm,
  onReject,
}: {
  tag: Tag;
  renaming: boolean;
  blockStart: boolean;
  confirming: boolean;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
  onStartRename: (tag: Tag) => void;
  onOpenSynonyms: (tag: Tag) => void;
  onOpenMerge: (tag: Tag) => void;
  onDelete: (tag: Tag) => void;
  onConfirm?: (tag: Tag) => void;
  onReject?: (tag: Tag) => void;
}) {
  const startBlocked = blockStart || confirming;
  return (
    <div className="hidden shrink-0 max-sm:flex [@media(pointer:coarse)]:flex">
      <MenuRoot>
        <MenuTrigger asChild>
          <IconButton
            ref={(node) => registerRefs(tag.id, { actionsButton: node })}
            label={t.tags.row.actions}
            size="sm"
            tooltip={false}
            aria-busy={confirming || undefined}
            disabled={renaming}
            onPointerDown={(event) => {
              // 確定の送信中は次の押下を無視する（行の「確定する」と同じ）。
              if (confirming) event.preventDefault();
            }}
            onKeyDown={(event) => {
              if (confirming && (event.key === "Enter" || event.key === " ")) {
                event.preventDefault();
              }
            }}
          >
            {confirming ? (
              <LoaderCircle className="animate-spin motion-reduce:animate-none" />
            ) : (
              <Ellipsis />
            )}
          </IconButton>
        </MenuTrigger>
        <MenuContent>
          {tag.tentative && (
            <MenuItem onSelect={() => onConfirm?.(tag)} disabled={confirming}>
              <Check />
              {t.tags.row.confirm}
            </MenuItem>
          )}
          <MenuItem onSelect={() => onStartRename(tag)} disabled={startBlocked}>
            <Pencil />
            {t.tags.row.rename}
          </MenuItem>
          <MenuItem onSelect={() => onOpenSynonyms(tag)}>
            <SynonymsIcon />
            {t.tags.row.synonymsButton}
          </MenuItem>
          <MenuItem onSelect={() => onOpenMerge(tag)} disabled={startBlocked}>
            <Merge />
            {t.tags.row.merge}
          </MenuItem>
          <MenuSeparator />
          {tag.tentative ? (
            <MenuItem
              tone="danger"
              onSelect={() => onReject?.(tag)}
              disabled={startBlocked}
            >
              <Ban />
              {t.tags.row.reject}
            </MenuItem>
          ) : (
            <MenuItem
              tone="danger"
              onSelect={() => onDelete(tag)}
              disabled={startBlocked}
            >
              <Trash2 />
              {t.tags.row.delete}
            </MenuItem>
          )}
        </MenuContent>
      </MenuRoot>
    </div>
  );
}
