import { Fragment, type ReactNode, useEffect, useId, useRef } from "react";

import { Button } from "@/ui/shadcn/button";
import { Separator } from "@/ui/shadcn/separator";
import { Skeleton } from "@/ui/shadcn/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";

// 移り先の一覧（区画）。ListPage の aside に置き、本体の移り先（日と月など）を 1 つ選ばせる。
// lg からは h2 の下の縦の一覧（ghost の sm の大きさ、幅いっぱいで左寄せ）で、最後の区切りの
// 後ろに action を置く。lg 未満では見出しを読み上げだけにし、outline の sm のチップを横に
// 送る 1 行にして、action は置かない。押している項目は届いたときに見える位置へ送る。
// 規則は web/registry/rules/patterns.md の Sections。

/** JumpListItem は移り先の 1 つである。 */
export interface JumpListItem {
  /** 選んだときに onValueChange へ渡す値。一覧の中で一意。 */
  value: string;
  /** 見える名前（「Tue, Oct 7」「September」）。 */
  label: ReactNode;
}

export interface JumpListProps {
  /** 見出し（「Jump to date」）。lg からは h2、lg 未満では読み上げだけ。 */
  title: string;
  /** 区切りで分けた項目のまとまり（日、月）。新しい順。空のまとまりは描かない。 */
  groups: readonly (readonly JumpListItem[])[];
  /** 押している項目の値。無ければ null。 */
  value: string | null;
  /** 項目を選ぶ。押している項目をもう一度押すと null。 */
  onValueChange: (value: string | null) => void;
  /** lg からだけ、最後の区切りの後ろに置く操作（ghost-destructive の sm の Button）。 */
  action?: ReactNode;
  /** 項目を読んでいる間。Skeleton の行を描く。 */
  loading?: boolean;
  /** 項目が 1 つも無いときの文言。lg から項目の位置に出し、lg 未満では帯ごと描かない。 */
  emptyText?: ReactNode;
  /** 項目の読み込みに失敗した。文言と「Retry」を項目の位置に出す。 */
  failed?: { message: ReactNode; retryLabel: ReactNode; onRetry: () => void };
}

export function JumpList({
  title,
  groups,
  value,
  onValueChange,
  action,
  loading = false,
  emptyText,
  failed,
}: JumpListProps) {
  const filled = groups.filter((group) => group.length > 0);
  const strip = useRef<HTMLDivElement>(null);
  const headingId = useId();
  const items = filled.flat();
  const itemsKey = items.map((item) => item.value).join(" ");

  // 押している項目を帯の見える位置へ送る（lg 未満の横に送る帯だけ。縦の欄では動かない）。
  useEffect(() => {
    const root = strip.current;
    if (root === null || value === null) return;
    const pressed = Array.from(
      root.querySelectorAll<HTMLElement>('[data-slot="toggle-group-item"]'),
    ).find((element) => element.dataset.value === value);
    if (pressed === undefined) return;
    root.scrollLeft = Math.max(
      0,
      pressed.offsetLeft - (root.clientWidth - pressed.offsetWidth) / 2,
    );
  }, [value, itemsKey]);

  let content: ReactNode;
  // lg 未満で帯を描かないとき（項目が無い）は、区画ごと lg からにする。
  let narrowHidden = false;
  if (failed !== undefined) {
    content = (
      <div className="flex items-center gap-2 lg:flex-col lg:items-start lg:gap-1">
        <p className="text-xs text-muted-foreground">{failed.message}</p>
        <Button variant="ghost" size="sm" onClick={failed.onRetry}>
          {failed.retryLabel}
        </Button>
      </div>
    );
  } else if (loading) {
    content = (
      <div className="flex gap-2 lg:flex-col" aria-hidden="true">
        {[0, 1, 2, 3].map((index) => (
          <Skeleton key={index} className="h-8 w-16 lg:w-full" />
        ))}
      </div>
    );
  } else if (items.length === 0) {
    narrowHidden = true;
    content = <p className="text-xs text-muted-foreground">{emptyText}</p>;
  } else {
    content = (
      <ToggleGroup
        ref={strip}
        type="single"
        size="sm"
        aria-label={title}
        value={value ?? ""}
        onValueChange={(next) => onValueChange(next === "" ? null : next)}
        className="w-full gap-2 overflow-x-auto lg:flex-col lg:items-stretch lg:gap-0.5 lg:overflow-visible"
      >
        {filled.map((group, index) => (
          <Fragment key={group[0]?.value ?? index}>
            {index > 0 && (
              <>
                <Separator orientation="vertical" className="h-6 lg:hidden" />
                <Separator className="my-1 hidden lg:block" />
              </>
            )}
            {group.map((item) => (
              <ToggleGroupItem
                key={item.value}
                value={item.value}
                data-value={item.value}
                className="flex-none rounded-md border border-input px-3 font-normal text-muted-foreground aria-checked:border-primary-active lg:w-full lg:justify-start lg:border-transparent lg:px-2 lg:aria-checked:border-transparent"
              >
                {item.label}
              </ToggleGroupItem>
            ))}
          </Fragment>
        ))}
      </ToggleGroup>
    );
  }

  return (
    <section
      data-slot="jump-list"
      aria-labelledby={headingId}
      className={narrowHidden ? "hidden flex-col gap-2 lg:flex" : "flex flex-col gap-2"}
    >
      <h2
        id={headingId}
        className="sr-only text-sm font-semibold text-foreground lg:not-sr-only"
      >
        {title}
      </h2>
      {content}
      {action !== undefined && (
        <div className="hidden flex-col gap-1 lg:flex">
          <Separator className="my-1" />
          {action}
        </div>
      )}
    </section>
  );
}
