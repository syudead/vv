import { ChevronLeft, ChevronRight } from "lucide-react";
import {
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { Link } from "react-router";

import { Button } from "@/ui/shadcn/button";
import { Skeleton } from "@/ui/shadcn/skeleton";

import { ErrorState } from "./error-state";

// 棚の行（区画）。見出しの行（題、前に置く印、題を開くリンク、右端の操作）の下に、横に送る
// 1 行のカードを置く。カードは card-0 の幅で gap-3 離し、送るとカードの始まりで止まる。md から
// ホバーできるポインタでは、行にポインタかフォーカスがある間、行の両端に前と次へ送る ghost の
// icon-sm のボタンを重ね、端に着いた側は描かない。行の中身はカード、読み込み中の形、失敗の
// どれか 1 つ。棚の間の間隔は ShelfRows が持つ。規則は web/registry/rules/patterns.md の
// Sections。

export interface ShelfRowsProps {
  /** 一覧の名前（「Shelves」）。支援技術に読まれる。 */
  label: string;
  /** ShelfRow を上から順に並べる。 */
  children: ReactNode;
}

/** ShelfRows は棚の行を gap-6 離して縦に並べる。行の中のカードの間より広く空ける。 */
export function ShelfRows({ label, children }: ShelfRowsProps) {
  return (
    <section data-slot="shelf-rows" aria-label={label} className="flex flex-col gap-6">
      {children}
    </section>
  );
}

export interface ShelfRowProps {
  /** 棚の題。h2 になる。 */
  title: ReactNode;
  /** 題の前に置く lucide の印（aria-hidden にして渡す）。 */
  leading?: ReactNode;
  /** 渡すと題がこの場所へのリンクになる。 */
  href?: string;
  /** 見出しの行の右端の操作（「See all」の link の sm、所有者の More）。 */
  actions?: ReactNode;
  /** 前へ送るボタンの読み上げ名。 */
  previousLabel: string;
  /** 次へ送るボタンの読み上げ名。 */
  nextLabel: string;
  /** 項目を読んでいる間の読み上げ名。渡すとカードの代わりに card-0 の Skeleton を並べる。 */
  loading?: string;
  /** 項目の読み込みに失敗した。カードの代わりに Alert と「Retry」を出す。 */
  failed?: { message: ReactNode; retryLabel: ReactNode; onRetry: () => void };
  /** カード。各子が card-0 の幅の 1 枚になる。 */
  children?: ReactNode;
  /** 行の要素（画面に入ったかを見張るのに使う）。 */
  ref?: Ref<HTMLElement>;
}

/** ScrollEnds は行が端に着いているかである。 */
interface ScrollEnds {
  start: boolean;
  end: boolean;
}

/**
 * useScrollEnds は横に送る行が始まりと終わりに着いているかを、送るたびと大きさが変わる
 * たびに読む。`content` が変わったとき（カードが届いたとき）も測り直す。
 */
function useScrollEnds(content: unknown) {
  const scroller = useRef<HTMLDivElement | null>(null);
  const [ends, setEnds] = useState<ScrollEnds>({ start: true, end: true });
  const measure = useCallback(() => {
    const element = scroller.current;
    if (element === null) return;
    const start = element.scrollLeft <= 1;
    const end = element.scrollLeft + element.clientWidth >= element.scrollWidth - 1;
    setEnds((previous) =>
      previous.start === start && previous.end === end ? previous : { start, end },
    );
  }, []);
  useEffect(() => {
    const element = scroller.current;
    if (element === null) return;
    measure();
    element.addEventListener("scroll", measure, { passive: true });
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => {
      element.removeEventListener("scroll", measure);
      observer.disconnect();
    };
  }, [measure, content]);
  return { scroller, ends };
}

/**
 * ShelfRow は棚 1 つである。前と次のボタンは行の見えている幅の分だけ送る。キーボードでは
 * カードへ Tab で移ると行が送られるので、ボタンは Tab の順に入れない。
 */
export function ShelfRow({
  title,
  leading,
  href,
  actions,
  previousLabel,
  nextLabel,
  loading,
  failed,
  children,
  ref,
}: ShelfRowProps) {
  const cards = loading === undefined && failed === undefined;
  const { scroller, ends } = useScrollEnds(cards ? children : null);
  const page = (direction: 1 | -1) => {
    const element = scroller.current;
    if (element === null) return;
    element.scrollBy({ left: direction * element.clientWidth, behavior: "smooth" });
  };

  return (
    <section data-slot="shelf-row" ref={ref} className="group/shelf flex flex-col gap-2">
      <div className="flex min-w-0 items-center gap-2">
        <h2 className="flex min-w-0 flex-1 text-base font-semibold text-foreground">
          <ShelfTitle title={title} leading={leading} href={href} />
        </h2>
        {actions !== undefined && (
          <div className="flex shrink-0 items-center gap-1">{actions}</div>
        )}
      </div>
      {loading !== undefined ? (
        <div role="status" aria-label={loading} aria-busy="true">
          <ShelfCardSkeletons />
        </div>
      ) : failed !== undefined ? (
        <ErrorState
          title={failed.message}
          retryLabel={failed.retryLabel}
          onRetry={failed.onRetry}
        />
      ) : (
        <div className="relative">
          <div
            ref={scroller}
            data-slot="shelf-row-cards"
            className="flex snap-x snap-mandatory gap-3 overflow-x-auto scrollbar-none *:w-card-0 *:shrink-0 *:snap-start"
          >
            {children}
          </div>
          {!ends.start && (
            <ShelfRowButton side="start" label={previousLabel} onPress={() => page(-1)} />
          )}
          {!ends.end && (
            <ShelfRowButton side="end" label={nextLabel} onPress={() => page(1)} />
          )}
        </div>
      )}
    </section>
  );
}

/** ShelfTitle は見出しの中身である。`href` があれば題と印をまとめて 1 つのリンクにする。 */
function ShelfTitle({
  title,
  leading,
  href,
}: Pick<ShelfRowProps, "title" | "leading" | "href">) {
  const content = (
    <>
      {leading}
      <span className="min-w-0 truncate">{title}</span>
    </>
  );
  if (href === undefined) {
    return (
      <span className="flex min-w-0 items-center gap-2 [&_svg]:size-4 [&_svg]:shrink-0">
        {content}
      </span>
    );
  }
  return (
    <Link
      to={href}
      className="flex min-w-0 items-center gap-2 rounded-sm transition-colors hover:text-primary [&_svg]:size-4 [&_svg]:shrink-0"
    >
      {content}
    </Link>
  );
}

const startButtonClass =
  "pointer-events-none absolute inset-y-0 left-0 hidden items-center opacity-0 transition-opacity group-focus-within/shelf:opacity-100 group-hover/shelf:opacity-100 md:[@media(hover:hover)]:flex";
const endButtonClass =
  "pointer-events-none absolute inset-y-0 right-0 hidden items-center opacity-0 transition-opacity group-focus-within/shelf:opacity-100 group-hover/shelf:opacity-100 md:[@media(hover:hover)]:flex";

/**
 * ShelfRowButton は行の端に重ねる送りのボタンである。md からホバーできるポインタだけで、
 * 行にポインタかフォーカスがある間に見せる。
 */
function ShelfRowButton({
  side,
  label,
  onPress,
}: {
  side: "start" | "end";
  label: string;
  onPress: () => void;
}) {
  return (
    <div className={side === "start" ? startButtonClass : endButtonClass}>
      <Button
        variant="ghost"
        size="icon-sm"
        tabIndex={-1}
        aria-label={label}
        onClick={onPress}
        className="pointer-events-auto bg-background/90 hover:bg-accent"
      >
        {side === "start" ? (
          <ChevronLeft aria-hidden="true" />
        ) : (
          <ChevronRight aria-hidden="true" />
        )}
      </Button>
    </div>
  );
}

/** shelfCardCount は棚の骨組みの 1 行に並べるカードの数である。広い幅でも右端で切れる数にする。 */
const shelfCardCount = 8;

/**
 * ShelfCardSkeletons は棚の行 1 つ分のカードの形（card-0 の幅、行の右端で切る）である。
 * ShelfRow の読み込み中と LoadingState の `shelves` が使う。
 */
export function ShelfCardSkeletons() {
  return (
    <div className="flex gap-3 overflow-hidden">
      {Array.from({ length: shelfCardCount }, (_, index) => (
        <div key={index} className="flex w-card-0 shrink-0 flex-col gap-2">
          <Skeleton className="aspect-video w-full" />
          <Skeleton className="h-4 w-3/4" />
        </div>
      ))}
    </div>
  );
}
