import { Shapes } from "lucide-react";
import type { Ref } from "react";

import type { TagKind } from "../api/tagKinds";
import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Spinner } from "../ui/shadcn/spinner";

/** KindChoice は「Set kind…」のメニューで選ぶ値である。種類か、種類を外す `none`。 */
export type KindChoice = TagKind | "none";

/**
 * TagSetKindMenu は選択バーの「Set kind…」とそのメニューである。1 つの `DropdownMenuRadioGroup` に
 * 名前の自然順の種類、区切り線、`No kind` を並べ、選ぶと確認なしで `onChoose` を呼ぶ。
 * 印は選んだタグがみな持つ種類（みな種類なしなら `No kind`）に付け、種類が混ざれば
 * どれにも付けない。種類が 1 つも無いときは、どこで作るかを伝える `DropdownMenuLabel` を
 * 上に置き、`No kind` だけを並べる。
 */
export default function TagSetKindMenu({
  kinds,
  checked,
  disabled,
  pending,
  title,
  describedBy,
  onChoose,
  buttonRef,
}: {
  /** 名前の自然順の種類。読み込み前は undefined。 */
  kinds: readonly TagKind[] | undefined;
  /** 印を付ける値（種類の id の文字列か `none`）。混ざっていれば undefined。 */
  checked: string | undefined;
  disabled: boolean;
  /** 送信中。アイコンを回す。 */
  pending: boolean;
  /** 押せない理由（選んだ数が上限を超える）。 */
  title: UiText | undefined;
  describedBy: string | undefined;
  onChoose: (choice: KindChoice) => void;
  buttonRef?: Ref<HTMLButtonElement>;
}) {
  const list = kinds ?? [];
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          ref={buttonRef}
          variant="ghost"
          size="sm"
          disabled={disabled}
          title={title}
          aria-describedby={describedBy}
        >
          {pending ? <Spinner aria-hidden="true" /> : <Shapes aria-hidden="true" />}
          <span>{t.tags.selection.setKind}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {kinds !== undefined && list.length === 0 && (
          <DropdownMenuLabel className="max-w-menu font-normal">
            {t.tags.selection.noKinds}
          </DropdownMenuLabel>
        )}
        {/* 項目はどれも 1 つの組にあり、区切り線は見た目の境だけである。 */}
        <DropdownMenuRadioGroup value={checked ?? ""}>
          {list.map((kind) => (
            <DropdownMenuRadioItem
              key={kind.id}
              value={String(kind.id)}
              onSelect={() => onChoose(kind)}
            >
              <span title={kind.name} className="max-w-chip-label min-w-0 truncate">
                {kind.name}
              </span>
            </DropdownMenuRadioItem>
          ))}
          {list.length > 0 && <DropdownMenuSeparator />}
          <DropdownMenuRadioItem value="none" onSelect={() => onChoose("none")}>
            {t.tags.kindFilter.none}
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
