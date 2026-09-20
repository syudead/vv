import { AlertCircle, FolderOpen, type LucideIcon, SearchX } from "lucide-react";
import type { ReactNode } from "react";

import Button from "../ui/Button";
import Skeleton from "../ui/Skeleton";

function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  tone = "neutral",
}: {
  icon: LucideIcon;
  title: string;
  description?: string;
  action?: ReactNode;
  tone?: "neutral" | "danger";
}) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-24 text-center animate-fade-in">
      <div
        className={
          "mb-5 flex size-16 items-center justify-center rounded-2xl " +
          (tone === "danger" ? "bg-danger-soft text-danger" : "bg-surface text-fg-muted")
        }
      >
        <Icon className="size-7" strokeWidth={1.5} />
      </div>
      <h2 className="text-lg font-semibold tracking-tight text-fg">{title}</h2>
      {description !== undefined && (
        <p className="mt-1.5 max-w-md text-sm text-fg-muted text-balance">
          {description}
        </p>
      )}
      {action !== undefined && <div className="mt-6 flex gap-2">{action}</div>}
    </div>
  );
}

export function EmptyLibrary({
  onScan,
  scanning,
}: {
  onScan: () => void;
  scanning: boolean;
}) {
  return (
    <EmptyState
      icon={FolderOpen}
      title="動画がまだありません"
      description="メディアフォルダに動画を置いて取り込むと、ここに並びます。"
      action={
        <Button variant="primary" onClick={onScan} disabled={scanning}>
          {scanning ? "取り込み中…" : "取り込む"}
        </Button>
      }
    />
  );
}

export function NoMatches({ query, onClear }: { query: string; onClear: () => void }) {
  return (
    <EmptyState
      icon={SearchX}
      title={`「${query}」に一致する動画はありません`}
      description="別の言葉で探すか、検索語を消してください。"
      action={<Button onClick={onClear}>検索語をクリア</Button>}
    />
  );
}

export function NoFilterMatches({ onReset }: { onReset: () => void }) {
  return (
    <EmptyState
      icon={SearchX}
      title="条件に一致する動画はありません"
      description="読み込み済みの範囲に該当がありません。絞り込みを緩めてください。"
      action={<Button onClick={onReset}>絞り込みを解除</Button>}
    />
  );
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

export function GridSkeleton({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, index) => (
        <div key={index} className="flex flex-col gap-2.5" aria-hidden="true">
          <Skeleton className="aspect-video w-full rounded-lg" />
          <div className="flex flex-col gap-1.5 px-0.5">
            <Skeleton className="h-4 w-4/5" />
            <Skeleton className="h-3 w-1/3" />
          </div>
        </div>
      ))}
    </>
  );
}
