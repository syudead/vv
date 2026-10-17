import { ListVideo, Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { type PlaylistList, listPlaylists, type PlaylistSummary } from "../api/playlists";
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

/** More は続きのページ、または古くなった一覧の最初のページの読み直しの状態である。 */
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
      /** 読んだページの一覧の版。続きのページの版がこれと違えば、並びが変わっている。 */
      version: string;
      /**
       * 読んだページが古い（作った後、または続きのページの版が違った後）。次の読みは最初の
       * ページで、読んだページと置き換える。
       */
      stale: boolean;
      more: More;
    };

/** readyPage は応答の 1 ページ目を、読んだページとして表す。 */
function readyPage(list: PlaylistList): Page {
  return {
    kind: "ready",
    items: list.items,
    total: list.total,
    nextCursor: list.nextCursor,
    version: list.version,
    stale: false,
    more: { kind: "idle" },
  };
}

/**
 * PlaylistsPage は所有者のプレイリストを最後に変えた順にカードの格子で並べ、新しく作らせる画面である。カードはライブラリのグループのカードの形
 * （PlaylistCard）で、格子はライブラリと同じ CardGrid である。
 *
 * 一覧はライブラリと同じく 1 ページずつ読む。格子の末尾の見張りが画面に
 * 近づいたら `nextCursor` で続きを読み、読んでいる間は LoadMoreRow の Spinner、失敗したら今の
 * カードを残して Retry つきの LoadMoreRow を出す。見出しの件数は応答の `total`（全部の数）である。
 *
 * 並びは更新日時の新しい順なので、読んでいる間に変わったプレイリストは前のページの側へ動き、
 * `nextCursor` の次からは返らない。続きのページの `version` が読んだページと違えばそれが起きたので、
 * 足さずに最初のページから読み直して置き換える。作った後も同じで、最初のページを読み直す。
 * 読み直しに失敗したら今のカードを残して Retry つきの LoadMoreRow を出し、Retry は最初のページを
 * もう一度求める。読み直しが終わるか失敗すると見張りは付け直され、続きはそこから読まれる。
 *
 * 最初のページは開いたときと作った後に読む。開いている間は自分では取り直さない。
 * 作ったプレイリストは応答で先頭に置いてから最初のページを読み直し、フォーカスをそのカードのリンクへ
 * 移す。同じ名前のプレイリストもそれぞれ 1 枚に出す。
 */
export default function PlaylistsPage() {
  const [page, setPage] = useState<Page>({ kind: "loading" });
  const [creating, setCreating] = useState(false);
  const [focusId, setFocusId] = useState<number | null>(null);
  const links = useRef(new Map<number, HTMLAnchorElement>());
  const pageRef = useRef(page);
  // 進行中の一覧の要求。同時に 1 つだけで、新しい要求や作った後の読み直しが来たら打ち切る。
  const inflight = useRef<AbortController | null>(null);

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

  const begin = useCallback(() => {
    inflight.current?.abort();
    const controller = new AbortController();
    inflight.current = controller;
    return controller;
  }, []);

  /** load は何も出していないときに最初のページを読む（開いたとき、Load failed の Retry）。 */
  const load = useCallback(() => {
    const controller = begin();
    listPlaylists({ signal: controller.signal }).then(
      (list) => {
        if (controller.signal.aborted) return;
        inflight.current = null;
        showPage(readyPage(list));
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        inflight.current = null;
        showPage({ kind: "failed" });
      },
    );
  }, [begin, showPage]);

  useEffect(() => {
    load();
    return () => {
      inflight.current?.abort();
    };
  }, [load]);

  /**
   * loadMore は、読んだページが古ければ最初のページを読み直して置き換え、そうでなければ
   * `nextCursor` の続きのページを読んで今のカードの後ろに足す。続きも読み直しも要らないとき、
   * 読んでいる間は何もしない。失敗した読みは Retry がもう一度求める。
   */
  const loadMore = useCallback(() => {
    const read = (cursor: string | undefined) => {
      const controller = begin();
      listPlaylists({ cursor, signal: controller.signal }).then(
        (list) => {
          const now = pageRef.current;
          if (controller.signal.aborted || now.kind !== "ready") return;
          if (cursor === undefined) {
            inflight.current = null;
            showPage(readyPage(list));
            return;
          }
          if (list.version !== now.version) {
            // 読んでいる間に並びが変わった。足すと動いたプレイリストが欠けるので、最初のページから
            // 読み直す。
            showPage({ ...now, stale: true, more: { kind: "loading" } });
            read(undefined);
            return;
          }
          inflight.current = null;
          showPage({
            ...now,
            items: [...now.items, ...list.items],
            total: list.total,
            nextCursor: list.nextCursor,
            more: { kind: "idle" },
          });
        },
        (error: unknown) => {
          const now = pageRef.current;
          if (isAborted(error) || controller.signal.aborted || now.kind !== "ready")
            return;
          inflight.current = null;
          showPage({ ...now, more: { kind: "failed", error } });
        },
      );
    };

    const current = pageRef.current;
    if (current.kind !== "ready" || current.more.kind === "loading") return;
    if (!current.stale && current.nextCursor === undefined) return;
    showPage({ ...current, more: { kind: "loading" } });
    read(current.stale ? undefined : current.nextCursor);
  }, [begin, showPage]);

  // --- 無限スクロール（ライブラリと同じ見張り） ---
  const sentinel = useRef<HTMLDivElement | null>(null);
  const canLoadMore =
    page.kind === "ready" &&
    page.more.kind === "idle" &&
    (page.stale || page.nextCursor !== undefined);
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
    // ページを読み終えるたび（nextCursor が変わる、読みが終わる・失敗する）に見張りを付け直し、
    // 見えたままならすぐ次のページを読む。
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
    // 読んだページは古くなった。先頭に置いて見せながら、最初のページを読み直して置き換える。
    showPage({
      kind: "ready",
      items: [playlist, ...rest],
      total: (current.kind === "ready" ? current.total : 0) + 1,
      nextCursor: current.kind === "ready" ? current.nextCursor : undefined,
      version: current.kind === "ready" ? current.version : "",
      stale: true,
      more: { kind: "idle" },
    });
    setCreating(false);
    setFocusId(playlist.id);
    loadMore();
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
          title={
            page.stale
              ? t.playlists.loadFailed
              : t.list.loadMoreFailed(errorText(page.more.error))
          }
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
