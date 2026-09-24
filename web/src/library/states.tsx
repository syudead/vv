import { AlertCircle, FolderOpen, type LucideIcon, SearchX } from "lucide-react";
import type { ReactNode } from "react";

import Button from "../ui/Button";
import Chip from "../ui/Chip";
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

/**
 * NoMatches は条件に一致する動画が無いときの状態である（ui-design.md「No-match state」）。
 * ライブラリとフォルダ画面で同じ部品を使う。conditions は効いている条件の名前
 * （「検索語「京都」」「未視聴」など）で、並べ替えは解除しないので含めない。
 */
export function NoMatches({
  conditions,
  note,
  onClear,
}: {
  conditions: string[];
  /** 条件のチップの下に添える一文（フォルダ画面の絞り込みだけのときなど）。 */
  note?: string;
  onClear: () => void;
}) {
  return (
    <EmptyState
      icon={SearchX}
      title="条件に一致する動画はありません"
      description={
        <>
          <ul
            aria-label="効いている条件"
            className="flex flex-wrap justify-center gap-1.5"
          >
            {conditions.map((condition) => (
              <li key={condition} className="flex max-w-full min-w-0">
                <Chip className="max-w-full" title={condition}>
                  <span className="min-w-0 truncate">{condition}</span>
                </Chip>
              </li>
            ))}
          </ul>
          {note !== undefined && <p className="mt-2">{note}</p>}
        </>
      }
      action={<Button onClick={onClear}>条件を解除</Button>}
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
