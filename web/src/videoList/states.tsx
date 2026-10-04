import { AlertCircle, FolderOpen, type LucideIcon, SearchX } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";

import { currentPath, loginPath } from "../auth/pageNavigation";
import { t, type UiText } from "../i18n";
import Button, { buttonClassName } from "../ui/legacy/Button";
import Skeleton from "../ui/Skeleton";

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
    <div className="mx-auto flex w-full max-w-lg flex-col items-center justify-center rounded-lg border border-border bg-card px-6 py-16 text-center animate-fade-in motion-reduce:animate-none">
      <Icon
        className={
          "mb-4 size-10 " + (tone === "danger" ? "text-destructive" : "text-primary")
        }
        strokeWidth={1.5}
        aria-hidden="true"
      />
      <h2 className="text-lg font-semibold text-foreground">{title}</h2>
      {description !== undefined &&
        (typeof description === "string" ? (
          <p className="mt-1.5 text-sm text-muted-foreground text-balance">
            {description}
          </p>
        ) : (
          <div className="mt-1.5 w-full text-sm text-muted-foreground text-balance">
            {description}
          </div>
        ))}
      {action !== undefined && <div className="mt-5 flex gap-2">{action}</div>}
    </div>
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
        <Link
          to={loginPath(currentPath(location))}
          className={buttonClassName("secondary")}
        >
          {t.list.guestEmpty.signIn}
        </Link>
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
          <Button variant="secondary" onClick={onSearch}>
            {t.list.changeSearch}
          </Button>
        )
      }
    />
  );
}

export function LoadFailed({ reason, onRetry }: { reason: UiText; onRetry: () => void }) {
  return (
    <EmptyState
      icon={AlertCircle}
      tone="danger"
      title={t.list.loadFailed}
      description={reason}
      action={<Button onClick={onRetry}>{t.common.retry}</Button>}
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
    <div className="flex flex-wrap items-center justify-center gap-2 rounded-md border border-destructive bg-destructive-soft px-3 py-2 text-sm text-destructive">
      <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
      <p>{t.list.loadMoreFailed(reason)}</p>
      <Button size="sm" onClick={onRetry}>
        {t.common.retry}
      </Button>
    </div>
  );
}

export function CardSkeleton({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, index) => (
        <div
          key={index}
          className="flex flex-col overflow-hidden rounded-lg bg-card shadow-card"
          aria-hidden="true"
        >
          <Skeleton className="aspect-video w-full rounded-none" />
          <div className="flex flex-col gap-1.5 px-3 pt-2 pb-3">
            <Skeleton className="h-4 w-4/5" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        </div>
      ))}
    </>
  );
}
