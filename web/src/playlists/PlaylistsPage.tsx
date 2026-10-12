import { ListVideo, Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { listPlaylists, type PlaylistSummary } from "../api/playlists";
import { t } from "../i18n";
import { CardGrid } from "../ui/patterns/card-grid";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import NewPlaylistDialog from "./NewPlaylistDialog";
import { PlaylistCard } from "./PlaylistCard";

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "ready"; items: readonly PlaylistSummary[] };

/**
 * PlaylistsPage は所有者のプレイリストを最後に変えた順にカードの格子で並べ、新しく作らせる画面である
 * （specs/044-playlists/ui-design.md「Playlists screen」）。カードはライブラリのグループのカードの形
 * （PlaylistCard）で、格子はライブラリと同じ CardGrid である。
 *
 * 一覧は開いたときと作った後に読む。開いている間は自分では取り直さない（research.md R-10）。
 * 作ったプレイリストは応答で先頭に置いてから一覧を読み直し、フォーカスをそのカードのリンクへ移す。
 * 同じ名前のプレイリストもそれぞれ 1 枚に出す（research.md R-11）。
 */
export default function PlaylistsPage() {
  const [page, setPage] = useState<Page>({ kind: "loading" });
  const [creating, setCreating] = useState(false);
  const [focusId, setFocusId] = useState<number | null>(null);
  const links = useRef(new Map<number, HTMLAnchorElement>());
  const pageRef = useRef(page);
  const fetching = useRef<AbortController | null>(null);

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
    const controller = new AbortController();
    fetching.current = controller;
    listPlaylists(controller.signal).then(
      (items) => {
        if (controller.signal.aborted) return;
        fetching.current = null;
        showPage({ kind: "ready", items });
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
    return () => fetching.current?.abort();
  }, [load]);

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
    showPage({ kind: "ready", items: [playlist, ...rest] });
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
            page.kind === "ready" && !empty
              ? t.playlists.count(page.items.length)
              : undefined
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
      {creating && (
        <NewPlaylistDialog onClose={() => setCreating(false)} onCreated={created} />
      )}
    </ListPage>
  );
}
