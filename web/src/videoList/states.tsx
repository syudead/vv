import { AlertCircle, FolderOpen, type LucideIcon, SearchX } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";

import { currentPath, loginPath } from "../auth/pageNavigation";
import Button, { buttonClassName } from "../ui/Button";
import Skeleton from "../ui/Skeleton";

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  tone = "neutral",
}: {
  icon: LucideIcon;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  tone?: "neutral" | "danger";
}) {
  return (
    <div className="mx-auto flex w-full max-w-lg flex-col items-center justify-center px-6 py-20 text-center animate-fade-in">
      <Icon
        className={
          "mb-4 size-10 " + (tone === "danger" ? "text-danger" : "text-fg-subtle")
        }
        strokeWidth={1.5}
      />
      <h2 className="text-lg font-semibold text-fg">{title}</h2>
      {description !== undefined &&
        (typeof description === "string" ? (
          <p className="mt-1.5 text-sm text-fg-muted text-balance">{description}</p>
        ) : (
          <div className="mt-1.5 w-full text-sm text-fg-muted text-balance">
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
      title="公開されている動画はありません"
      description="ログインすると、すべての動画を見られます"
      action={
        <Link
          to={loginPath(currentPath(location))}
          className={buttonClassName("secondary")}
        >
          ログイン
        </Link>
      }
    />
  );
}

/** NoMatches は条件に一致する動画が無いことだけを示す。 */
export function NoMatches() {
  return <EmptyState icon={SearchX} title="条件に一致する動画はありません" />;
}

export function LoadFailed({ reason, onRetry }: { reason: string; onRetry: () => void }) {
  return (
    <EmptyState
      icon={AlertCircle}
      tone="danger"
      title="一覧を取得できません"
      description={reason}
      action={<Button onClick={onRetry}>再試行</Button>}
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
  reason: string;
  onRetry: () => void;
}) {
  return (
    <div className="flex items-center justify-center gap-2 text-sm text-danger">
      <p>続きを取得できません: {reason}</p>
      <Button size="sm" onClick={onRetry}>
        再試行
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
          className="flex flex-col overflow-hidden rounded-md bg-surface"
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
