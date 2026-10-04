import { FolderOpen, FolderX, SearchX } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";

import { currentPath, loginPath } from "../auth/pageNavigation";
import { t, type UiText } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { Button } from "../ui/shadcn/button";
import { FOLDERS_ROOT } from "./folderPath";

// フォルダ画面の一覧の状態（web/registry/rules/patterns.md の States）。どれも一覧ページの
// 骨格の本体の位置に置く。題は見出し（h2）にし、読み上げソフトで見出しから状態へ飛べる
// ようにする（状態の部品の題は見出しの要素を持たないため、ここで包む）。

/** FolderEmpty は見出しつきの空の状態である。 */
export function FolderEmpty({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode;
  title: UiText;
  description?: UiText;
  action?: ReactNode;
}) {
  return (
    <EmptyState
      icon={icon}
      title={<h2>{title}</h2>}
      description={description}
      action={action}
    />
  );
}

/** FolderNotFound は開いたフォルダが無いときの状態で、フォルダの最上位へ戻る入口を置く。 */
export function FolderNotFound() {
  return (
    <FolderEmpty
      icon={<FolderX aria-hidden="true" />}
      title={t.folders.notFound.title}
      description={t.folders.notFound.description}
      action={
        <Button asChild size="sm">
          <Link to={FOLDERS_ROOT}>{t.folders.notFound.back}</Link>
        </Button>
      }
    />
  );
}

/** NoMatches は条件に一致する動画が無いときの状態である。 */
export function NoMatches() {
  return (
    <FolderEmpty
      icon={<SearchX aria-hidden="true" />}
      title={t.list.noMatches}
      description={t.list.noMatchesHint}
    />
  );
}

/**
 * GuestEmpty はゲストに公開の動画が1本も無いときの状態である。取り込みや設定の
 * 代わりに、ログインへの入口を置く（specs/016-single-account-auth/ui-design.md
 * 「Guest degradation」）。
 */
export function GuestEmpty() {
  const location = useLocation();
  return (
    <FolderEmpty
      icon={<FolderOpen aria-hidden="true" />}
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

/** LoadFailed は最初の読み込みに失敗したときの状態で、同じ要求をやり直させる。 */
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
 * MoreRow は格子の後ろに置く追加読み込みの行である。読み込み中は回転の印、続きの
 * 取得に失敗したら読んだ分を残したまま失敗とやり直しを出す。どちらでもなければ何も出さない。
 */
export function MoreRow({
  loadingMore,
  error,
  onRetry,
}: {
  loadingMore: boolean;
  /** 続きの取得の失敗の理由。読んだ分があるときだけ渡す。 */
  error: UiText | null;
  onRetry: () => void;
}) {
  if (error !== null) {
    return (
      <LoadMoreRow
        status="failed"
        title={t.list.loadMoreFailed(error)}
        retryLabel={t.common.retry}
        onRetry={onRetry}
      />
    );
  }
  return loadingMore ? <LoadMoreRow status="loading" label={t.common.loading} /> : null;
}

/** CardsLoading は読み込み中のカードの格子である。読み込み後の格子と同じ大きさで並べる。 */
export function CardsLoading({ zoom, count }: { zoom: Zoom; count: number }) {
  return <LoadingState label={t.list.loading} layout="grid" size={zoom} count={count} />;
}

/**
 * ResultCount は検索結果の件数の行である（ライブラリと同じ要約行）。件数の変化を
 * polite で読み上げる。
 */
export function ResultCount({ children }: { children: ReactNode }) {
  return (
    <p
      role="status"
      aria-live="polite"
      className="text-center text-xs text-muted-foreground tabular-nums"
    >
      {children}
    </p>
  );
}
