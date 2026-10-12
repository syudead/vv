import { ListVideo, Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { listPlaylists, type PlaylistSummary } from "../api/playlists";
import { errorText, t } from "../i18n";
import { CardGrid } from "../ui/patterns/card-grid";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import NewPlaylistDialog from "./NewPlaylistDialog";
import { PlaylistCard } from "./PlaylistCard";

/** More は続きのページの読み込みの状態である。 */
type More = { kind: "idle" } | { kind: "loading" } | { kind: "failed"; error: unknown };

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | {
      kind: "ready";
      items: readonly PlaylistSummary[];
      /** 全部のプレイリストの数（ページングとは独立）。見出しの件数に出す。 */
      total: number;
      /** 続きのページの鍵。無ければもう続きは無い。 */
      nextCursor?: string;
      more: More;
    };

/**
 * PlaylistsPage は所有者のプレイリストを最後に変えた順にカードの格子で並べ、新しく作らせる画面である
 * （specs/044-playlists/ui-design.md「Playlists screen」）。カードはライブラリのグループのカードの形
 * （PlaylistCard）で、格子はライブラリと同じ CardGrid である。
 *
 * 一覧はライブラリと同じく 1 ページずつ読む（ui-design.md「Paging」）。格子の末尾の見張りが画面に
 * 近づいたら `nextCursor` で続きを読み、読んでいる間は LoadMoreRow の Spinner、失敗したら今の
 * カードを残して Retry つきの LoadMoreRow を出す。見出しの件数は応答の `total`（全部の数）である。
 *
 * 最初のページは開いたときと作った後に読む。開いている間は自分では取り直さない（research.md R-10）。
 * 作ったプレイリストは応答で先頭に置いてから最初のページを読み直し、フォーカスをそのカードのリンクへ
 * 移す。同じ名前のプレイリストもそれぞれ 1 枚に出す（research.md R-11）。
 */
export default function PlaylistsPage() {
  const [page, setPage] = useState<Page>({ kind: "loading" });
  const [creating, setCreating] = useState(false);
  const [focusId, setFocusId] = useState<number | null>(null);
  const links = useRef(new Map<number, HTMLAnchorElement>());
  const pageRef = useRef(page);
  // 最初のページの要求。続きの要求や作った後の読み直しが来たら打ち切る。
  const fetching = useRef<AbortController | null>(null);
  // 続きのページの要求。最初のページを読み直すときに打ち切る。
  const fetchingMore = useRef<AbortController | null>(null);

  const showPage = useCallback((next: Page) => {
    pageRef.current = next;
    setPage(next);
  }, []);

  useEffect(() => {
    const previous = document.title;
    document.title = t.playlists.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  const load = useCallback(() => {
    fetching.current?.abort();
    fetchingMore.current?.abort();
    fetchingMore.current = null;
    const controller = new AbortController();
    fetching.current = controller;
    listPlaylists({ signal: controller.signal }).then(
      (list) => {
        if (controller.signal.aborted) return;
        fetching.current = null;
        showPage({
          kind: "ready",
          items: list.items,
          total: list.total,
          nextCursor: list.nextCursor,
          more: { kind: "idle" },
        });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        fetching.current = null;
        // 作った後の読み直しの失敗では、今の一覧を残す。
        if (pageRef.current.kind !== "ready") showPage({ kind: "failed" });
      },
    );
  }, [showPage]);

  useEffect(() => {
    load();
    return () => {
      fetching.current?.abort();
      fetchingMore.current?.abort();
    };
  }, [load]);

  /**
   * loadMore は `nextCursor` の続きのページを読み、今のカードの後ろに足す。続きが無いとき、
   * 読んでいる間、最初のページを読んでいる間は何もしない。失敗した同じページは Retry が
   * もう一度求める。
   */
  const loadMore = useCallback(() => {
    const current = pageRef.current;
    if (
      current.kind !== "ready" ||
      current.nextCursor === undefined ||
      current.more.kind === "loading" ||
      fetching.current !== null
    )
      return;
    const cursor = current.nextCursor;
    const controller = new AbortController();
    fetchingMore.current = controller;
    showPage({ ...current, more: { kind: "loading" } });
    listPlaylists({ cursor, signal: controller.signal }).then(
      (list) => {
        const now = pageRef.current;
        if (controller.signal.aborted || now.kind !== "ready") return;
        fetchingMore.current = null;
        // ページの境で重なったプレイリスト（読んでいる間に更新されたもの）は 1 枚にする。
        const known = new Set(now.items.map((item) => item.id));
        const added = list.items.filter((item) => !known.has(item.id));
        showPage({
          kind: "ready",
          items: [...now.items, ...added],
          total: list.total,
          nextCursor: list.nextCursor,
          more: { kind: "idle" },
        });
      },
      (error: unknown) => {
        const now = pageRef.current;
        if (isAborted(error) || controller.signal.aborted || now.kind !== "ready") return;
        fetchingMore.current = null;
        showPage({ ...now, more: { kind: "failed", error } });
      },
    );
  }, [showPage]);

  // --- 無限スクロール（ライブラリと同じ見張り） ---
  const sentinel = useRef<HTMLDivElement | null>(null);
  const canLoadMore =
    page.kind === "ready" && page.nextCursor !== undefined && page.more.kind === "idle";
  const nextCursor = page.kind === "ready" ? page.nextCursor : undefined;
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !canLoadMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
    // nextCursor が変わるたびに見張りを付け直し、見えたままならすぐ次のページを読む。
  }, [canLoadMore, loadMore, nextCursor]);

  // 窓を閉じた後で新しいカードのリンクへフォーカスを移す。窓は閉じるときに開く前の要素へ
  // フォーカスを戻すので（setTimeout 0）、それより後に移す。
  useEffect(() => {
    if (focusId === null) return;
    const timer = setTimeout(() => {
      links.current.get(focusId)?.focus();
      setFocusId(null);
    }, 0);
    return () => clearTimeout(timer);
  }, [focusId]);

  function created(playlist: PlaylistSummary) {
    const current = pageRef.current;
    const rest =
      current.kind === "ready"
        ? current.items.filter((item) => item.id !== playlist.id)
        : [];
    showPage({
      kind: "ready",
      items: [playlist, ...rest],
      total: (current.kind === "ready" ? current.total : 0) + 1,
      nextCursor: current.kind === "ready" ? current.nextCursor : undefined,
      more: { kind: "idle" },
    });
    setCreating(false);
    setFocusId(playlist.id);
    load();
  }

  function retry() {
    showPage({ kind: "loading" });
    load();
  }

  const empty = page.kind === "ready" && page.items.length === 0;
  const newButton = (
    <Button size="sm" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" />
      {t.playlists.new}
    </Button>
  );

  return (
    <ListPage
      header={
        <PageHeader
          title={t.playlists.title}
          count={
            page.kind === "ready" && !empty ? t.playlists.count(page.total) : undefined
          }
          // 空のときは本体の「New playlist」だけを出し、主操作を 1 つにする。
          actions={empty ? undefined : newButton}
        />
      }
    >
      {page.kind === "loading" && <LoadingState label={t.list.loading} layout="grid" />}
      {page.kind === "failed" && (
        <ErrorState
          title={t.playlists.loadFailed}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      )}
      {empty && (
        <EmptyState
          icon={<ListVideo aria-hidden="true" />}
          title={t.playlists.empty.title}
          description={t.playlists.empty.description}
          action={newButton}
        />
      )}
      {page.kind === "ready" && !empty && (
        <CardGrid>
          {page.items.map((playlist) => (
            <PlaylistCard
              key={playlist.id}
              playlist={playlist}
              linkRef={(node) => {
                if (node === null) links.current.delete(playlist.id);
                else links.current.set(playlist.id, node);
              }}
            />
          ))}
        </CardGrid>
      )}
      {page.kind === "ready" && page.more.kind === "loading" && (
        <LoadMoreRow status="loading" label={t.list.loading} />
      )}
      {page.kind === "ready" && page.more.kind === "failed" && (
        <LoadMoreRow
          status="failed"
          title={t.list.loadMoreFailed(errorText(page.more.error))}
          retryLabel={t.common.retry}
          onRetry={loadMore}
        />
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
      {creating && (
        <NewPlaylistDialog onClose={() => setCreating(false)} onCreated={created} />
      )}
    </ListPage>
  );
}
