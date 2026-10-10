import type { ReactNode, Ref } from "react";

import { cn } from "@/lib/cn";

// 日ごとの時間軸（区画）。1 本の縦の線の上に行を並べ、まとまりの見出し（日）を sm からは
// 線の左の列に、狭い幅では行の上に置く。線・点・見出しの列・時刻の列・行の中の並びと
// すべての間隔はこの区画が持ち、画面は見出しの文言と行の中身（時刻・サムネイル・文字・
// 操作）を渡すだけにする。規則は web/registry/rules/patterns.md の Sections。

export interface TimelineProps {
  /** 一覧の名前（「Watch history」）。支援技術に読まれる。 */
  label: string;
  /** TimelineGroup を新しい順に並べる。 */
  children: ReactNode;
}

function TimelineRoot({ label, children }: TimelineProps) {
  return (
    <ol data-slot="timeline" aria-label={label} className="flex flex-col">
      {children}
    </ol>
  );
}

/** Dot は線の上の点である。置く高さは呼び手が決める。 */
function Dot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "absolute -left-1 size-2 rounded-full bg-muted-foreground",
        className,
      )}
    />
  );
}

export interface TimelineGroupProps {
  /** 見出しの 1 行目（「Today」「Tuesday」）。 */
  label: ReactNode;
  /** 見出しの 2 行目（「Oct 7」）。 */
  detail?: ReactNode;
  /** TimelineItem を並べる。 */
  children: ReactNode;
}

/**
 * TimelineGroup は見出しと、その行である。見出しは h2 で、sm からは線の左の列に 2 行で、
 * 狭い幅では線の右に 1 行（`Today · Oct 10`）で自分の点と並ぶ。まとまりの間は線だけが続く。
 */
function TimelineGroup({ label, detail, children }: TimelineGroupProps) {
  return (
    <li
      data-slot="timeline-group"
      className="group/timeline-group grid sm:grid-cols-timeline-group"
    >
      <div className="relative border-l border-border pb-1 pl-4 sm:border-l-0 sm:pt-2 sm:pr-3 sm:pb-0 sm:pl-0">
        <Dot className="top-1.5 sm:hidden" />
        <h2 className="flex flex-wrap items-baseline gap-x-1 sm:flex-col sm:gap-0.5">
          <span className="text-sm font-semibold text-foreground">{label}</span>
          {detail !== undefined && (
            <>
              {" "}
              <span
                aria-hidden="true"
                className="text-xs text-muted-foreground sm:hidden"
              >
                ·
              </span>{" "}
              <span className="text-xs text-muted-foreground">{detail}</span>
            </>
          )}
        </h2>
      </div>
      <ol className="flex flex-col border-l border-border pb-6 group-last/timeline-group:pb-0">
        {children}
      </ol>
    </li>
  );
}

/** TimelineMainProps は押せる行の本体を描く関数に渡す値である。 */
export interface TimelineMainProps {
  /** 本体の並びと押せる面の見た目。描く要素にそのまま付ける。 */
  className: string;
  /** 時刻・サムネイル・文字。描く要素の中にそのまま置く。 */
  children: ReactNode;
}

export interface TimelineItemProps {
  /** 行の時刻（「9:42 PM」）。時刻の列に置く。 */
  time: ReactNode;
  /** サムネイル。列の幅いっぱいに描く（`VideoThumbnail` は既定で w-full）。 */
  media: ReactNode;
  /** 文字の列: 題名と補いの行。 */
  children: ReactNode;
  /**
   * 行の本体（時刻・サムネイル・文字）を押せる要素にする関数。リンクにするときは
   * `({ className, children }) => <Link className={className}>{children}</Link>`。
   * 無ければ押せない行で、ホバーの塗りも無い。
   */
  main?: (props: TimelineMainProps) => ReactNode;
  /** 本体の外の操作（「Resume」）。狭い幅では文字の列の下、sm からは行の端の手前。 */
  action?: ReactNode;
  /** 行の端の静かなボタン（×）。狭い幅では時刻の行の端。 */
  remove?: ReactNode;
  ref?: Ref<HTMLLIElement>;
}

const mainLayout =
  "col-span-2 col-start-1 row-span-3 row-start-1 grid grid-cols-subgrid grid-rows-subgrid items-start sm:col-span-3 sm:row-span-1";

/**
 * TimelineItem は線の上の 1 行である。sm からは時刻・サムネイル・文字・操作・端のボタンを
 * 1 行に、狭い幅では時刻と端のボタンの行の下にサムネイルと文字、その下に操作を置く。
 * 本体と操作は別の要素なので、操作を押しても本体は押されない。
 */
function TimelineItem({
  time,
  media,
  children,
  main,
  action,
  remove,
  ref,
}: TimelineItemProps) {
  const content = (
    <>
      <span className="col-span-2 self-center text-xs text-muted-foreground tabular-nums sm:col-span-1 sm:self-start sm:pt-0.5">
        {time}
      </span>
      <span className="row-span-2 min-w-0 sm:row-span-1">{media}</span>
      <span className="flex min-w-0 flex-col gap-1">{children}</span>
    </>
  );
  return (
    <li
      ref={ref}
      data-slot="timeline-item"
      className="relative grid grid-cols-timeline-item items-start gap-x-3 gap-y-1 py-2 pl-4 sm:grid-cols-timeline-item-wide"
    >
      <Dot className="top-5 sm:top-3" />
      {main !== undefined ? (
        main({
          className: cn(mainLayout, "rounded-md transition-colors hover:bg-accent"),
          children: content,
        })
      ) : (
        <div className={mainLayout}>{content}</div>
      )}
      {action !== undefined && (
        <div className="relative col-start-2 row-start-3 flex sm:col-start-4 sm:row-start-1">
          {action}
        </div>
      )}
      {remove !== undefined && (
        <div className="col-start-3 row-start-1 flex justify-end sm:col-start-5">
          {remove}
        </div>
      )}
    </li>
  );
}

/** Timeline は時間軸の全体で、`Timeline.Group` と `Timeline.Item` を持つ。 */
const Timeline = Object.assign(TimelineRoot, {
  Group: TimelineGroup,
  Item: TimelineItem,
});

export { Timeline, TimelineGroup, TimelineItem };
