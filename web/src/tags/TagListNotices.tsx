import { CircleAlert, RefreshCw, RotateCw } from "lucide-react";

import { t, type UiText } from "../i18n";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { Alert, AlertAction, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";

/**
 * TagLoadingMore は一覧の末尾の続きの読み込み中である（specs/036-tag-admin-scale/
 * ui-design.md「Loading more」）。表の後ろに状態の部品 `LoadMoreRow` の Spinner の行を
 * 置く（web/registry/rules/patterns.md の States）。
 */
export function TagLoadingMore() {
  return <LoadMoreRow status="loading" label={t.tags.loadingMore} />;
}

/**
 * TagLoadMoreFailed は続きの読み込みの失敗である。読み込んだ行は残り、「Retry」が
 * 同じカーソルで読み直す（ui-design.md「Loading more」）。
 */
export function TagLoadMoreFailed({
  reason,
  onRetry,
}: {
  reason: UiText;
  onRetry: () => void;
}) {
  return (
    <LoadMoreRow
      status="failed"
      title={t.tags.loadMoreFailed(reason)}
      retryLabel={t.common.retry}
      onRetry={onRetry}
    />
  );
}

/**
 * TagListChanged は、続きの応答の `totalAll` が画面の値と違ったときの知らせである。
 * 失敗ではないので destructive にせず既定の `Alert` にする。「Reload」で先頭から読み直す
 * （ui-design.md「Loading more」、research.md R-11）。
 */
export function TagListChanged({ onReload }: { onReload: () => void }) {
  return (
    <Alert role="status">
      <RefreshCw aria-hidden="true" />
      <AlertTitle>{t.tags.listChanged}</AlertTitle>
      <AlertAction>
        <Button variant="outline" size="sm" onClick={onReload}>
          {t.tags.reloadList}
        </Button>
      </AlertAction>
    </Alert>
  );
}

/**
 * TagLoadFailed は、一覧を持たないまま先頭のページを読めなかったときに表の位置に出す
 * 失敗である（状態の部品 `ErrorState` と同じ destructive の `Alert`）。題は見出し（h2）に
 * し、「Retry」は読み直している間も押せないまま残して、フォーカスを失わせない。
 */
export function TagLoadFailed({
  pending,
  onRetry,
}: {
  pending: boolean;
  onRetry: () => void;
}) {
  return (
    <Alert data-state="error" variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>
        <h2>{t.tags.loadFailed}</h2>
      </AlertTitle>
      <AlertAction>
        <Button variant="outline" size="sm" onClick={onRetry} disabled={pending}>
          <RotateCw aria-hidden="true" />
          {t.common.retry}
        </Button>
      </AlertAction>
    </Alert>
  );
}

/**
 * TagStaleList は、一覧を持っている間に先頭のページを読めなかったときに表の上に
 * 出す失敗である。残した行は今の条件と合わないかもしれないことを伝える
 * （ui-design.md「Stale list」）。
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
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{t.tags.staleList(reason)}</AlertTitle>
      <AlertAction>
        <Button variant="outline" size="sm" onClick={onRetry} disabled={pending}>
          <RotateCw aria-hidden="true" />
          {t.common.retry}
        </Button>
      </AlertAction>
    </Alert>
  );
}
