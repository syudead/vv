import type { ReactNode, Ref } from "react";

import { cn } from "@/lib/cn";

// 1 行のカードの一覧（区画）。枠線のある muted の面のカードを gap-2 で縦に重ね、カードの
// 色（tone）は呼ぶ側が決める。カードの端には時刻と操作の決まった幅の列を置き、時刻と操作が
// 一覧の下まで揃う。lg からは 1 行（印・題・チップ・右に寄せた結果・時刻・操作）、lg 未満では
// 2 行（印と題、その下にチップ・結果・短い時刻。操作は右で 2 行にまたがる）。
// 余白・面・線・列の幅はこの区画が持ち、画面は中身を渡すだけにする
//
// 規則は web/registry/rules/patterns.md の Sections。

export type CardListTone = "default" | "primary" | "destructive" | "warning";

const toneClass: Record<CardListTone, string> = {
  default: "border-border bg-muted",
  primary: "border-primary bg-primary-soft",
  destructive: "border-destructive-strong bg-destructive-soft",
  warning: "border-warning bg-warning-soft",
};

export interface CardListProps {
  /** 一覧の名前（「Activity log」）。支援技術に読まれる。 */
  label: string;
  /** CardListItem を並べる。 */
  children: ReactNode;
}

function CardListRoot({ label, children }: CardListProps) {
  return (
    <ul data-slot="card-list" aria-label={label} className="flex flex-col gap-2">
      {children}
    </ul>
  );
}

export interface CardListItemProps {
  /** カードの色。既定は default（border と muted）。 */
  tone?: CardListTone;
  /** 行の頭の状態の印（size-5 のアイコン）。 */
  icon: ReactNode;
  /** 題（ファイル名など）。1 行で、幅が足りないとき最初に切る。 */
  title: ReactNode;
  /** 種類のチップ（Badge）。切らない。 */
  badge?: ReactNode;
  /** 結果。lg からは右に寄せ、題の次に切る。中の文字の要素に truncate を付ける。 */
  detail?: ReactNode;
  /** 時刻（lg から、activity-time の幅の列）。 */
  time: ReactNode;
  /** lg 未満で結果の後ろに置く短い時刻（「15m」）。 */
  timeShort: ReactNode;
  /** ghost の icon-sm のボタン。決まった幅の端の列に置く。 */
  actions?: ReactNode;
  ref?: Ref<HTMLLIElement>;
}

/**
 * CardListItem は一覧の 1 枚のカードである。最小の高さは activity-card、横の余白は px-4。
 */
function CardListItem({
  tone = "default",
  icon,
  title,
  badge,
  detail,
  time,
  timeShort,
  actions,
  ref,
}: CardListItemProps) {
  return (
    <li
      ref={ref}
      data-slot="card-list-item"
      data-tone={tone}
      className={cn(
        "flex min-h-activity-card min-w-0 items-center gap-3 rounded-lg border px-4 py-2",
        toneClass[tone],
      )}
    >
      <span data-slot="card-list-icon" className="flex shrink-0 items-center">
        {icon}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1 lg:flex-row lg:items-center lg:gap-3">
        <div
          data-slot="card-list-title"
          className="min-w-0 truncate text-sm font-medium text-foreground lg:flex-1"
        >
          {title}
        </div>
        <div className="flex min-w-0 items-center gap-2 lg:max-w-2/3 lg:shrink-0 lg:gap-3">
          {badge !== undefined && <span className="flex shrink-0">{badge}</span>}
          <div
            data-slot="card-list-detail"
            className="flex min-w-0 flex-1 items-center gap-2 text-sm text-muted-foreground lg:justify-end"
          >
            {detail}
          </div>
          <span
            data-slot="card-list-time-short"
            className="shrink-0 text-sm whitespace-nowrap text-muted-foreground lg:hidden"
          >
            {timeShort}
          </span>
        </div>
        <span
          data-slot="card-list-time"
          className="hidden min-w-activity-time shrink-0 text-right text-sm whitespace-nowrap text-muted-foreground lg:block"
        >
          {time}
        </span>
      </div>
      <div
        data-slot="card-list-actions"
        className="flex min-w-16 shrink-0 items-center justify-end"
      >
        {actions}
      </div>
    </li>
  );
}

/** CardList は一覧の全体で、`CardList.Item` を持つ。 */
const CardList = Object.assign(CardListRoot, { Item: CardListItem });

export { CardList, CardListItem };
