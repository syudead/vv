import {
  Ban,
  Check,
  Ellipsis,
  Merge,
  Pencil,
  Tags as SynonymsIcon,
  Trash2,
} from "lucide-react";
import { type ComponentProps, memo, type ReactNode, type Ref } from "react";
import { Link } from "react-router";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Checkbox } from "../ui/shadcn/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Spinner } from "../ui/shadcn/spinner";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import TentativeMark from "../ui/TentativeMark";
import RowNameInput from "./RowNameInput";
import TagKindLabel from "./TagKindLabel";
import type { TagFieldError } from "./tagNameField";

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
  index,
  measureRef,
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
  onSelect,
}: {
  /** 一覧の中の位置。仮想化が行を測るときに読む（`data-index`）。 */
  index: number;
  /** 仮想化の行の測り（`virtualizer.measureElement`）。 */
  measureRef: Ref<HTMLTableRowElement>;
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
  onSelect?: (tag: Tag, selected: boolean) => void;
}) {
  return (
    <TableRow
      ref={measureRef}
      data-index={index}
      data-tag-id={tag.id}
      data-state={selected && !renaming ? "selected" : undefined}
    >
      <TableCell>
        {/*
          押す範囲はチェックを中央に置いた size-8 の正方形（タッチでも外さない）。
          包みの余白を押しても同じチェックを切り替える。改名中の行は選ばない
          （改名の確定で並びの別の位置へ動きうるため。ui-design.md「Row checkbox」）。
        */}
        <div
          className="-my-1.5 flex size-8 items-center justify-center"
          onClick={(event) => {
            if (event.target !== event.currentTarget || renaming) return;
            onSelect?.(tag, !selected);
          }}
        >
          <Checkbox
            checked={selected && !renaming}
            onCheckedChange={(next) => onSelect?.(tag, next === true)}
            aria-label={t.tags.row.select(tag.name)}
            disabled={renaming}
          />
        </div>
      </TableCell>
      {/*
        名前の列は残りの幅をすべて取り（w-full）、長い名前は省略する（max-w-0 で表の幅を
        押し広げない）。
      */}
      <TableCell className="w-full max-w-0 whitespace-normal">
        {renaming ? (
          <RowNameInput
            idPrefix="tag-rename"
            rowId={tag.id}
            name={tag.name}
            label={t.tags.row.renameLabel(tag.name)}
            pending={pending}
            error={error}
            onSubmit={(name) => onSubmitRename(tag, name)}
            onCancel={() => onCancelRename(tag)}
            onDraftChange={onDraftChange}
          />
        ) : (
          <>
            {/*
              名前は省略されても、仮の目印は shrink-0 で名前の後ろに残す
              （ui-design.md「Tentative mark」）。Link は文字の幅に縮むので、`after:` の
              疑似要素を包み（relative）いっぱいに広げ、名前の列の空白を押しても絞り込んだ
              一覧が開くようにする。目印は relative で疑似要素の上に置き、Tooltip を
              受けられるようにする。
            */}
            <span className="relative flex min-w-0 items-center gap-1">
              <Link
                ref={(node) => registerRefs(tag.id, { nameLink: node })}
                to={`/?tag=${String(tag.id)}`}
                title={tag.name}
                aria-label={t.tags.row.open(tag.name)}
                className="block min-w-0 truncate font-medium text-foreground after:absolute after:inset-0 hover:text-primary"
              >
                {tag.name}
              </Link>
              {tag.tentative && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="relative inline-flex shrink-0">
                      <TentativeMark size="row" />
                      <span className="sr-only">{t.tags.tentative}</span>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>{t.tags.tentative}</TooltipContent>
                </Tooltip>
              )}
              <TagKindLabel kind={tag.kind} />
            </span>
            {tag.synonyms.length > 0 && (
              <p
                title={t.tags.row.synonyms(tag.synonyms.join(" · "))}
                className="truncate text-xs text-muted-foreground"
              >
                {t.tags.row.synonyms(tag.synonyms.join(" · "))}
              </p>
            )}
          </>
        )}
      </TableCell>
      <TableCell className="text-right text-muted-foreground tabular-nums">
        {t.tags.row.videoCount(tag.videoCount)}
      </TableCell>
      <TableCell>
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
        <div className="flex items-center justify-end gap-1 max-sm:hidden [@media(pointer:coarse)]:hidden">
          {/*
            確定したタグの行も「確定する」の分の幅を空けておき、操作の並びを全部の行で
            そろえる（specs/036-tag-admin-scale/ui-design.md「Column header」）。
          */}
          {!tag.tentative && <span aria-hidden="true" className="size-8 shrink-0" />}
          {tag.tentative && (
            <RowIconButton
              label={t.tags.row.confirm}
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
              {confirming ? <Spinner aria-hidden="true" /> : <Check aria-hidden="true" />}
            </RowIconButton>
          )}
          <RowIconButton
            ref={(node) => registerRefs(tag.id, { renameButton: node })}
            label={t.tags.row.rename}
            onClick={() => onStartRename(tag)}
            disabled={renaming || blockStart || confirming}
          >
            <Pencil aria-hidden="true" />
          </RowIconButton>
          <RowIconButton
            ref={(node) => registerRefs(tag.id, { synonymsButton: node })}
            label={t.tags.row.synonymsButton}
            onClick={() => onOpenSynonyms(tag)}
            disabled={renaming}
          >
            <SynonymsIcon aria-hidden="true" />
          </RowIconButton>
          <RowMoreMenu
            buttonRef={(node) => registerRefs(tag.id, { menuButton: node })}
            label={t.tags.row.more}
            disabled={renaming || blockStart || confirming}
          >
            <DropdownMenuItem onSelect={() => onOpenMerge(tag)}>
              <Merge aria-hidden="true" />
              {t.tags.row.merge}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            {tag.tentative ? (
              <DropdownMenuItem variant="destructive" onSelect={() => onReject?.(tag)}>
                <Ban aria-hidden="true" />
                {t.tags.row.reject}
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem variant="destructive" onSelect={() => onDelete(tag)}>
                <Trash2 aria-hidden="true" />
                {t.tags.row.delete}
              </DropdownMenuItem>
            )}
          </RowMoreMenu>
        </div>
      </TableCell>
    </TableRow>
  );
}

/**
 * RowIconButton は行の操作の、文字を持たないボタンである（ghost の icon-sm と、同じ
 * 名前の Tooltip。web/registry/rules/components.md「Button」）。
 */
export function RowIconButton({
  label,
  ...props
}: Omit<ComponentProps<typeof Button>, "variant" | "size" | "aria-label"> & {
  label: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={label} {...props} />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * RowMoreMenu は行の「More actions」（`Ellipsis` の ghost の icon-sm と同じ名前の Tooltip）と、
 * それが開くメニューである。タグの行と人物・団体の行（`web/src/credits/CreditRow`）が使う。
 */
export function RowMoreMenu({
  buttonRef,
  label,
  disabled,
  children,
}: {
  buttonRef: Ref<HTMLButtonElement>;
  label: string;
  disabled: boolean;
  /** メニューの項目。 */
  children: ReactNode;
}) {
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              ref={buttonRef}
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              disabled={disabled}
            >
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * CompactActions は、タッチの端末と `sm` 未満の幅で行の右端に出す「Actions」の
 * メニューである。項目は文字を持ち、行のアイコンのボタン（`RowIconButton`）と「その他の操作」の項目と
 * 同じ振る舞いをする（ui-design.md「Actions on touch and narrow widths」）。
 * マウスの端末で `sm` 以上では CSS で隠す（`matchMedia` は読まない）。
 *
 * DOM では行のアイコンのボタンの一群より前に置く。どちらか一方だけが描かれるので
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
    <div className="hidden justify-end max-sm:flex [@media(pointer:coarse)]:flex">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            ref={(node) => registerRefs(tag.id, { actionsButton: node })}
            variant="ghost"
            size="icon-sm"
            aria-label={t.tags.row.actions}
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
              <Spinner aria-hidden="true" />
            ) : (
              <Ellipsis aria-hidden="true" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {tag.tentative && (
            <DropdownMenuItem onSelect={() => onConfirm?.(tag)} disabled={confirming}>
              <Check aria-hidden="true" />
              {t.tags.row.confirm}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => onStartRename(tag)} disabled={startBlocked}>
            <Pencil aria-hidden="true" />
            {t.tags.row.rename}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onOpenSynonyms(tag)}>
            <SynonymsIcon aria-hidden="true" />
            {t.tags.row.synonymsButton}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onOpenMerge(tag)} disabled={startBlocked}>
            <Merge aria-hidden="true" />
            {t.tags.row.merge}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          {tag.tentative ? (
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => onReject?.(tag)}
              disabled={startBlocked}
            >
              <Ban aria-hidden="true" />
              {t.tags.row.reject}
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => onDelete(tag)}
              disabled={startBlocked}
            >
              <Trash2 aria-hidden="true" />
              {t.tags.row.delete}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
