import { ListVideo, Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { isAborted } from "../api/client";
import { listPlaylists, type PlaylistSummary } from "../api/playlists";
import { formatDateTime, formatRelative, t } from "../i18n";
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../ui/shadcn/table";
import NewPlaylistDialog from "./NewPlaylistDialog";

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "ready"; items: readonly PlaylistSummary[] };

/** playlistPath はプレイリストの画面の場所である。 */
function playlistPath(id: number): string {
  return `/playlists/${String(id)}`;
}

/**
 * PlaylistsPage は所有者のプレイリストを最後に変えた順に並べ、新しく作らせる画面である
 * （specs/044-playlists/ui-design.md「Playlists screen」）。
 *
 * 一覧は開いたときと作った後に読む。開いている間は自分では取り直さない（research.md R-10）。
 * 作ったプレイリストは応答で先頭に置いてから一覧を読み直し、フォーカスをその名前のリンクへ移す。
 * 同じ名前のプレイリストもそれぞれ 1 行に出す（research.md R-11）。
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

  // 窓を閉じた後で新しい行の名前へフォーカスを移す。窓は閉じるときに開く前の要素へ
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
      {page.kind === "loading" && <LoadingState label={t.list.loading} layout="table" />}
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
        <DataTable label={t.playlists.table}>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="pl-3">{t.playlists.columns.name}</TableHead>
              <TableHead className="w-list-number pr-4 text-right">
                {t.playlists.columns.items}
              </TableHead>
              <TableHead className="hidden w-list-date pr-3 text-right md:table-cell">
                {t.playlists.columns.updated}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {page.items.map((playlist) => (
              <PlaylistRow
                key={playlist.id}
                playlist={playlist}
                linkRef={(node) => {
                  if (node === null) links.current.delete(playlist.id);
                  else links.current.set(playlist.id, node);
                }}
              />
            ))}
          </TableBody>
        </DataTable>
      )}
      {creating && (
        <NewPlaylistDialog onClose={() => setCreating(false)} onCreated={created} />
      )}
    </ListPage>
  );
}

function PlaylistRow({
  playlist,
  linkRef,
}: {
  playlist: PlaylistSummary;
  linkRef: (node: HTMLAnchorElement | null) => void;
}) {
  return (
    <TableRow data-playlist-id={playlist.id} className="relative">
      <TableCell className="w-full max-w-0 pl-3">
        {/* リンクの after: を行（relative）いっぱいに広げ、行のどこを押しても開くようにする。 */}
        <Link
          ref={linkRef}
          to={playlistPath(playlist.id)}
          aria-label={t.playlists.rowLabel(playlist.name, playlist.itemCount)}
          className="flex min-w-0 items-center gap-2 rounded-sm text-sm font-medium text-foreground after:absolute after:inset-0 hover:text-primary"
        >
          <ListVideo
            aria-hidden="true"
            className="size-4 shrink-0 text-muted-foreground"
          />
          <span title={playlist.name} className="min-w-0 truncate">
            {playlist.name}
          </span>
        </Link>
      </TableCell>
      <TableCell className="w-list-number pr-4 text-right text-xs text-muted-foreground tabular-nums">
        {t.playlists.items(playlist.itemCount)}
      </TableCell>
      <TableCell className="hidden w-list-date pr-3 text-right text-xs text-muted-foreground tabular-nums md:table-cell">
        <span title={formatDateTime(playlist.updatedAt)}>
          {formatRelative(playlist.updatedAt)}
        </span>
      </TableCell>
    </TableRow>
  );
}
