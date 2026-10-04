import { FolderOpen, type LucideIcon, SearchX } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";

import { currentPath, loginPath } from "../auth/pageNavigation";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { EmptyState as EmptyStateBlock } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";

// 一覧の状態（空・失敗・読み込み中）。画面の型の状態のブロック（web/src/ui/patterns）を、
// 一覧の文言で埋めて使う（web/registry/rules/patterns.md の States）。題は見出し（h2）に
// して、画面の見出しの並びに入れる。

/**
 * EmptyState は本体の位置に出す空の状態である。`tone="danger"` は印を destructive の色に
 * する（画面ごと出せないとき）。
 */
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  tone = "neutral",
}: {
  icon: LucideIcon;
  title: UiText;
  description?: ReactNode;
  action?: ReactNode;
  tone?: "neutral" | "danger";
}) {
  return (
    <EmptyStateBlock
      icon={
        <Icon
          aria-hidden="true"
          className={cn(tone === "danger" && "text-destructive")}
        />
      }
      title={<h2>{title}</h2>}
      description={description}
      action={action}
    />
  );
}

/**
 * GuestEmpty はゲストに公開の動画が1本も無いときの状態である。取り込みや設定の
 * 代わりに、ログインへの入口を置く（specs/016-single-account-auth/ui-design.md
 * 「Guest degradation」）。ライブラリとフォルダ画面の最上位で同じ文言を使う。
 */
export function GuestEmpty() {
  const location = useLocation();
  return (
    <EmptyState
      icon={FolderOpen}
      title={t.list.guestEmpty.title}
      description={t.list.guestEmpty.description}
      action={
        <Button asChild size="sm">
          <Link to={loginPath(currentPath(location))}>{t.list.guestEmpty.signIn}</Link>
        </Button>
      }
    />
  );
}

/** NoMatches は条件に一致しない理由と次の操作を示す。 */
export function NoMatches({ onSearch }: { onSearch?: () => void }) {
  return (
    <EmptyState
      icon={SearchX}
      title={t.list.noMatches}
      description={t.list.noMatchesHint}
      action={
        onSearch && (
          <Button size="sm" onClick={onSearch}>
            {t.list.changeSearch}
          </Button>
        )
      }
    />
  );
}

/** LoadFailed は最初のページを取得できなかったときに本体の位置に出す失敗である。 */
export function LoadFailed({ reason, onRetry }: { reason: UiText; onRetry: () => void }) {
  return (
    <ErrorState
      title={<h2>{t.list.loadFailed}</h2>}
      description={reason}
      retryLabel={t.common.retry}
      onRetry={onRetry}
    />
  );
}

/**
 * LoadMoreFailed は続きのページを取得できなかったときに一覧の下へ出す一行である。
 * 読み込んだ分はそのまま残し、続きだけを取り直させる。
 */
export function LoadMoreFailed({
  reason,
  onRetry,
}: {
  reason: UiText;
  onRetry: () => void;
}) {
  return (
    <LoadMoreRow
      status="failed"
      title={t.list.loadMoreFailed(reason)}
      retryLabel={t.common.retry}
      onRetry={onRetry}
    />
  );
}

/** CardSkeleton はカードの格子（Grid）の中に並べる、読み込み中のカードの形である。 */
export function CardSkeleton({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, index) => (
        <div key={index} className="flex flex-col gap-2" aria-hidden="true">
          <Skeleton className="aspect-video w-full" />
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-3 w-1/2" />
        </div>
      ))}
    </>
  );
}
