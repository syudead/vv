import { Ellipsis, IdCard, Merge, Pencil, Trash2 } from "lucide-react";
import { memo, type Ref } from "react";
import { Link } from "react-router";

import type { Credit, CreditKind } from "../api/credits";
import { t } from "../i18n";
import RowNameInput from "../tags/RowNameInput";
import type { TagFieldError } from "../tags/tagNameField";
import { RowIconButton, RowMoreMenu, type TagRowRefs } from "../tags/TagRow";
import { creditNameRules } from "../ui/CreditCommand";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { kindTexts, secondLine } from "./creditRows";

/** CreditRowHandlers は行へ渡す操作である。行の再描画を減らすため、同じ関数を渡し続ける。 */
export interface CreditRowHandlers {
  onStartRename: (credit: Credit) => void;
  onCancelRename: (credit: Credit) => void;
  onSubmitRename: (credit: Credit, name: string) => void;
  onOpenDetails: (credit: Credit) => void;
  onOpenMerge: (credit: Credit) => void;
  onDelete: (credit: Credit) => void;
  onDraftChange: () => void;
}

/** creditRowIdAttribute は行が id を持つ属性である（`useVirtualTagRows` の `rowIdAttribute`）。 */
export const creditRowIdAttribute = "data-credit-id";

/**
 * CreditRow は人物・団体の管理画面の一覧の 1 行である（specs/047-video-work-info/
 * ui-design.md「People and organizations screens」）。タグの行（`TagRow`）と同じ形で、
 * チェックの列と仮の目印が無く、2 つ目のボタンが「Edit details」である。名前は
 * `/?person=<id>`（団体は `/?organization=<id>`）へのリンクで、その下に親と別名の 1 行を
 * 置く。改名中は名前の列だけを入力に置き換える。タッチの端末と `sm` 未満では操作を
 * 「Actions」1 つにまとめる（CSS だけで出し分ける）。
 *
 * 一覧は見えている行だけを描くので `React.memo` で包む。呼び出し元は props を安定させる。
 */
export default memo(CreditRow);

function CreditRow({
  kind,
  index,
  measureRef,
  credit,
  renaming,
  pending,
  blockStart,
  error,
  registerRefs,
  onStartRename,
  onCancelRename,
  onSubmitRename,
  onOpenDetails,
  onOpenMerge,
  onDelete,
  onDraftChange,
}: CreditRowHandlers & {
  kind: CreditKind;
  /** 一覧の中の位置。仮想化が行を測るときに読む（`data-index`）。 */
  index: number;
  measureRef: Ref<HTMLTableRowElement>;
  credit: Credit;
  renaming: boolean;
  pending: boolean;
  /** ほかの行の作成・改名が送信中で、この行では新しく改名や削除を始められない。 */
  blockStart: boolean;
  error: TagFieldError | null;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
}) {
  const line = secondLine(credit);
  const parameter = kind === "person" ? "person" : "organization";
  const mergeLabel = kindTexts(kind).mergeInto;

  return (
    <TableRow ref={measureRef} data-index={index} data-credit-id={credit.id}>
      {/*
        名前の列は残りの幅をすべて取り（w-full）、長い名前は省略する（max-w-0 で表の幅を
        押し広げない）。
      */}
      <TableCell className="w-full max-w-0 whitespace-normal">
        {renaming ? (
          <RowNameInput
            idPrefix={`${parameter}-rename`}
            rowId={credit.id}
            name={credit.name}
            rules={creditNameRules}
            label={t.credits.row.renameLabel(credit.name)}
            pending={pending}
            error={error}
            onSubmit={(name) => onSubmitRename(credit, name)}
            onCancel={() => onCancelRename(credit)}
            onDraftChange={onDraftChange}
          />
        ) : (
          <>
            {/*
              Link は文字の幅に縮むので、`after:` の疑似要素を包み（relative）いっぱいに
              広げ、名前の列の空白を押しても絞り込んだ一覧が開くようにする（タグの行と同じ）。
            */}
            <span className="relative flex min-w-0 items-center">
              <Link
                ref={(node) => registerRefs(credit.id, { nameLink: node })}
                to={`/?${parameter}=${String(credit.id)}`}
                title={credit.name}
                aria-label={t.credits.row.open(credit.name)}
                className="block min-w-0 truncate font-medium text-foreground after:absolute after:inset-0 hover:text-primary"
              >
                {credit.name}
              </Link>
            </span>
            {line !== null && (
              <p title={line} className="truncate text-xs text-muted-foreground">
                {line}
              </p>
            )}
          </>
        )}
      </TableCell>
      <TableCell className="w-16 text-right text-muted-foreground tabular-nums">
        {t.credits.row.videoCount(credit.videoCount)}
      </TableCell>
      <TableCell>
        <div className="hidden justify-end max-sm:flex [@media(pointer:coarse)]:flex">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                ref={(node) => registerRefs(credit.id, { actionsButton: node })}
                variant="ghost"
                size="icon-sm"
                aria-label={t.credits.row.actions}
                disabled={renaming}
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onSelect={() => onStartRename(credit)}
                disabled={blockStart}
              >
                <Pencil aria-hidden="true" />
                {t.credits.row.rename}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onOpenDetails(credit)}>
                <IdCard aria-hidden="true" />
                {t.credits.row.details}
              </DropdownMenuItem>
              <MenuEnd
                mergeLabel={mergeLabel}
                blockStart={blockStart}
                onMerge={() => onOpenMerge(credit)}
                onDelete={() => onDelete(credit)}
              />
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="flex items-center justify-end gap-1 max-sm:hidden [@media(pointer:coarse)]:hidden">
          <RowIconButton
            ref={(node) => registerRefs(credit.id, { renameButton: node })}
            label={t.credits.row.rename}
            onClick={() => onStartRename(credit)}
            disabled={renaming || blockStart}
          >
            <Pencil aria-hidden="true" />
          </RowIconButton>
          <RowIconButton
            ref={(node) => registerRefs(credit.id, { synonymsButton: node })}
            label={t.credits.row.details}
            onClick={() => onOpenDetails(credit)}
            disabled={renaming}
          >
            <IdCard aria-hidden="true" />
          </RowIconButton>
          <RowMoreMenu
            buttonRef={(node) => registerRefs(credit.id, { menuButton: node })}
            label={t.credits.row.more}
            disabled={renaming || blockStart}
          >
            <MenuEnd
              mergeLabel={mergeLabel}
              blockStart={false}
              onMerge={() => onOpenMerge(credit)}
              onDelete={() => onDelete(credit)}
            />
          </RowMoreMenu>
        </div>
      </TableCell>
    </TableRow>
  );
}

/** MenuEnd はメニューの末尾の「Merge into another …」、区切り、「Delete…」である。 */
function MenuEnd({
  mergeLabel,
  blockStart,
  onMerge,
  onDelete,
}: {
  mergeLabel: string;
  blockStart: boolean;
  onMerge: () => void;
  onDelete: () => void;
}) {
  return (
    <>
      <DropdownMenuItem onSelect={onMerge} disabled={blockStart}>
        <Merge aria-hidden="true" />
        {mergeLabel}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem variant="destructive" onSelect={onDelete} disabled={blockStart}>
        <Trash2 aria-hidden="true" />
        {t.credits.row.delete}
      </DropdownMenuItem>
    </>
  );
}
