import { AlertCircle, RefreshCw } from "lucide-react";

import { t, type UiText } from "../i18n";
import Button from "../ui/legacy/Button";
import Skeleton from "../ui/Skeleton";

/**
 * TagLoadingMore は一覧の末尾の続きの読み込み中である（specs/036-tag-admin-scale/
 * ui-design.md「Loading more」）。初回の読み込み中と同じ形の行の `Skeleton` を 3 つ
 * 置き、読み上げには `role="status"` で伝える。
 */
export function TagLoadingMore() {
  return (
    <div className="px-2 py-2">
      <div className="space-y-2" aria-hidden="true">
        {Array.from({ length: 3 }, (_, index) => (
          <Skeleton key={index} className="h-10" />
        ))}
      </div>
      <p role="status" className="sr-only">
        {t.tags.loadingMore}
      </p>
    </div>
  );
}

/**
 * TagLoadMoreFailed は続きの読み込みの失敗である。読み込んだ行は残り、「Retry」が
 * 同じカーソルで読み直す（ui-design.md「Loading more」）。ライブラリの
 * `LoadMoreFailed` と同じ箱。
 */
export function TagLoadMoreFailed({
  reason,
  onRetry,
}: {
  reason: UiText;
  onRetry: () => void;
}) {
  return (
    <div className="px-2">
      <div
        role="alert"
        className="my-3 flex flex-wrap items-center gap-2 rounded-md border border-danger bg-danger-soft px-3 py-2 text-sm text-danger"
      >
        <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
        <p>{t.tags.loadMoreFailed(reason)}</p>
        <Button size="sm" onClick={onRetry}>
          {t.common.retry}
        </Button>
      </div>
    </div>
  );
}

/**
 * TagListChanged は、続きの応答の `totalAll` が画面の値と違ったときの知らせである。
 * 失敗ではないので danger にせず中立の箱にする。「Reload」で先頭から読み直す
 * （ui-design.md「Loading more」、research.md R-11）。
 */
export function TagListChanged({ onReload }: { onReload: () => void }) {
  return (
    <div className="px-2">
      <div
        role="status"
        className="my-3 flex flex-wrap items-center gap-2 rounded-md border border-border-strong bg-elevated px-3 py-2 text-sm text-fg"
      >
        <RefreshCw aria-hidden="true" className="size-4 shrink-0" />
        <p>{t.tags.listChanged}</p>
        <Button size="sm" onClick={onReload}>
          {t.tags.reloadList}
        </Button>
      </div>
    </div>
  );
}

/**
 * TagStaleList は、一覧を持っている間に先頭のページを読めなかったときに帯の中に
 * 出す箱である。残した行は今の条件と合わないかもしれないことを、スクロールした
 * 位置でも見える場所で伝える（ui-design.md「Stale list」）。
 */
export function TagStaleList({
  reason,
  pending,
  onRetry,
}: {
  reason: UiText;
  pending: boolean;
  onRetry: () => void;
}) {
  return (
    <div
      role="alert"
      className="mt-2 flex flex-wrap items-center gap-2 rounded-md border border-danger bg-danger-soft px-3 py-2 text-sm text-danger"
    >
      <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
      <p>{t.tags.staleList(reason)}</p>
      <Button size="sm" onClick={onRetry} disabled={pending}>
        {t.common.retry}
      </Button>
    </div>
  );
}
