import { FolderOpen } from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useLocation } from "react-router";

import type { FolderRef, FolderScope } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { useFolderListing } from "../api/useFolderListing";
import { useVideos } from "../api/useVideos";
import { useScan } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import Button from "../ui/Button";
import { hasConditions } from "../videoList/listCriteria";
import { EmptyState, LoadFailed } from "../videoList/states";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import Breadcrumbs from "./Breadcrumbs";
import FolderContents from "./FolderContents";
import FolderSearchResults from "./FolderSearchResults";
import FolderToolbar from "./FolderToolbar";
import { breadcrumbsFor, folderKey, rootDisplayName } from "./folderPath";
import { FolderNotFound } from "./layout";
import { useArrival } from "./useArrival";
import { useConditions } from "./useConditions";
import { useFolderTagsRow } from "./useFolderTagsRow";

/** FolderView はフォルダ1件の中身（直下の子フォルダと動画、または検索結果）を並べる。 */
export default function FolderView({ folder }: { folder: FolderRef }) {
  const location = useLocation();
  const {
    criteria,
    zoom,
    searchField,
    changeSort,
    shuffle,
    changeWatch,
    changePlayable,
    commitQuery,
    clearAll,
    changeZoom: saveZoom,
  } = useConditions();
  const scan = useScan();
  const { refresh: refreshScan } = scan;
  const searching = criteria.query !== "";
  // 検索語があるときはフォルダとその配下すべてを対象にする。無ければ直下だけを絞る
  // （Plan の Structural Decisions 8、contracts/list-url.md §2）。direct は既定なので
  // 送らない（URL・要求を今までと同じ形に保つ）。
  const scope: FolderScope | undefined = searching ? "subtree" : undefined;

  // --- 再生画面から戻ったときは控えから復元する（要件 10） ---
  const key = folderKey(folder);
  const [restored] = useState(() => {
    const held = takeListSnapshot({ ...criteria, folder: key });
    return held?.folderListing === undefined ? undefined : held;
  });
  const heading = useArrival(restored !== undefined);
  const listing = useFolderListing(folder, restored?.folderListing);
  const videos = useVideos(
    {
      query: criteria.query,
      watch: criteria.watch,
      playable: criteria.playable,
      sort: criteria.sort,
      seed: criteria.seed,
      scope,
    },
    restored,
    folder,
  );

  // ホバープレビューは同時に 1 件だけ（ライブラリと同じ）。一覧や倍率が変わったら止める。
  const { resetPreview, cardProps: preview } = usePreviewCoordination();
  useEffect(() => {
    resetPreview();
  }, [resetPreview, videos.items, zoom]);
  // 倍率を変えても読んでいた位置を保つ（ライブラリと同じ）。
  const { listRef, capture } = useZoomAnchor(zoom);
  const changeZoom = useCallback(
    (next: typeof zoom) => {
      capture();
      saveZoom(next);
    },
    [capture, saveZoom],
  );

  const tagsRow = useFolderTagsRow();

  const summary = listing.data?.folder;
  const children = listing.data?.folders ?? [];
  const rootName = summary === undefined ? undefined : rootDisplayName(summary.rootPath);
  const name =
    summary?.name ?? (folder.path === "" ? rootName : folder.path.split("/").at(-1));
  const backTo = `${location.pathname}${location.search}`;

  // --- スクロール位置の復元 ---
  const pendingScroll = useRef(restored?.scrollY);
  useEffect(() => {
    const previous = history.scrollRestoration;
    history.scrollRestoration = "manual";
    return () => {
      history.scrollRestoration = previous;
    };
  }, []);
  useLayoutEffect(() => {
    const top = pendingScroll.current;
    if (top === undefined || (videos.items.length === 0 && children.length === 0)) return;
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
    // 初回の描画では TopBarPortal がツールバーを本文の流れに置き、直後にトップバーへ
    // 移す。その分だけ本文が縮み、スクロールの追従で位置がずれるので、描画の前に
    // もう一度合わせる。
    requestAnimationFrame(() => window.scrollTo({ top, behavior: "auto" }));
  }, [children.length, videos.items.length]);

  // --- 取り込みが終わったら控えを捨てて読み直す（Edge Case「取り込み中」） ---
  // 表示を始めた時点で終わっていた取り込みは、読み込んだ中身に反映済みである。
  const knownScanId = useRef(
    restored === undefined ? scan.finished?.id : restored.scanId,
  );
  // 読み直しの途中の中身は、終わった取り込みを反映していない。子フォルダと動画の
  // 両方が読み終わるまでは控えを取らず、戻ったときに読み直させる。
  const reloadPending = useRef(false);
  useEffect(() => {
    if (!listing.loading && !videos.loading) reloadPending.current = false;
  }, [listing.loading, videos.loading]);
  const { reload: reloadListing } = listing;
  const { reload: reloadVideos } = videos;
  useEffect(() => refreshScan(), [refreshScan]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    reloadPending.current = true;
    clearListSnapshot();
    reloadListing();
    reloadVideos();
  }, [reloadListing, reloadVideos, scan.finished]);

  const saveSnapshot = useCallback(() => {
    if (listing.data === null || reloadPending.current) return;
    saveListSnapshot(
      { ...criteria, folder: key },
      {
        items: videos.items,
        total: videos.total,
        cursor: videos.cursor,
        hasMore: videos.hasMore,
        scrollY: window.scrollY,
        scanId: knownScanId.current,
        folderListing: listing.data,
      },
    );
  }, [
    criteria,
    key,
    listing.data,
    videos.cursor,
    videos.hasMore,
    videos.items,
    videos.total,
  ]);

  // --- 無限スクロール（ライブラリと同じ観測点） ---
  const sentinel = useRef<HTMLDivElement | null>(null);
  const { hasMore, loadMore } = videos;
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !hasMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, loadMore]);

  const noVideosAtAll =
    !listing.loading &&
    !videos.loading &&
    summary !== undefined &&
    !searching &&
    !hasConditions(criteria) &&
    children.length === 0 &&
    videos.items.length === 0 &&
    videos.error === null;

  let body: ReactNode;
  if (listing.notFound || videos.notFound) {
    // 検索中にフォルダが無くなった（listFolderVideos が 404）ときも、一致なしでは
    // なく「このフォルダは見つかりません」を出す（list-api.md §5）。
    body = <FolderNotFound />;
  } else if (listing.error !== null) {
    body = <LoadFailed reason={listing.error} onRetry={listing.reload} />;
  } else if (searching) {
    // 検索結果（ui-design.md「Search results」）。
    body = (
      <FolderSearchResults
        folder={folder}
        videos={videos}
        zoom={zoom}
        backTo={backTo}
        preview={preview}
        tagsRow={tagsRow}
      />
    );
  } else if (noVideosAtAll) {
    body = (
      <EmptyState
        icon={FolderOpen}
        title="このフォルダにはまだ動画がありません"
        description="取り込むと、ここに並びます。"
        action={
          <Button variant="primary" onClick={scan.start} disabled={scan.running}>
            {scan.running ? "取り込み中…" : "取り込む"}
          </Button>
        }
      />
    );
  } else {
    // 直下だけを対象にする通常表示（絞り込みだけのときも同じ形。ui-design.md「Filter only」）。
    body = (
      <FolderContents
        criteria={criteria}
        listingLoading={listing.loading}
        childFolders={children}
        videos={videos}
        zoom={zoom}
        backTo={backTo}
        preview={preview}
        tagsRow={tagsRow}
      />
    );
  }

  return (
    <>
      <h1 ref={heading} tabIndex={-1} className="sr-only">
        {name ?? "フォルダ"}
      </h1>
      <TopBarPortal>
        <FolderToolbar
          query={criteria.query}
          onQueryCommit={commitQuery}
          searchRef={searchField}
          searchLabel={`${name ?? "フォルダ"}の中を検索`}
          searchPlaceholder="このフォルダ内を検索"
          sort={criteria.sort}
          onSortChange={changeSort}
          onShuffle={shuffle}
          watch={criteria.watch}
          onWatchChange={changeWatch}
          playable={criteria.playable}
          onPlayableChange={changePlayable}
          canClear={hasConditions(criteria)}
          onClear={clearAll}
          zoom={zoom}
          onZoomChange={changeZoom}
        />
      </TopBarPortal>
      <Breadcrumbs
        crumbs={
          // 見つからなかったフォルダでは登録フォルダの名前が分からないので、その段を出さない。
          listing.notFound || videos.notFound || listing.error !== null
            ? breadcrumbsFor(folder, undefined).filter((crumb) => crumb !== undefined)
            : breadcrumbsFor(folder, rootName)
        }
        suffix={searching && !videos.notFound ? "内を検索中" : undefined}
      />
      {/* 中身を押す直前（再生画面・子フォルダへ移る直前）の状態を控える。 */}
      <div ref={listRef} onClick={saveSnapshot} className="flex flex-col gap-3">
        {body}
      </div>
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </>
  );
}
