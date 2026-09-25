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
import { useFolderListing, useRootFolderName } from "../api/useFolderListing";
import { useVideos } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { useScan } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import Button from "../ui/Button";
import { hasConditions } from "../videoList/listCriteria";
import { EmptyState, LoadFailed } from "../videoList/states";
import Breadcrumbs from "./Breadcrumbs";
import FolderContents from "./FolderContents";
import FolderSearchResults from "./FolderSearchResults";
import FolderToolbar from "./FolderToolbar";
import { breadcrumbsFor, folderKey, rootFolderName } from "./folderPath";
import { FolderNotFound } from "./layout";
import { useArrival } from "./useArrival";
import { useConditions } from "./useConditions";

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
    changeZoom,
  } = useConditions();
  const scan = useScan();
  const owner = useAudience() === "owner";
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

  const summary = listing.data?.folder;
  const children = listing.data?.folders ?? [];
  // 登録フォルダの表示名。絶対パスが無い（ゲストの）応答で子フォルダを開いているときは、
  // この応答からは分からない（登録フォルダそのものなら name がそれである）ので、
  // 登録フォルダそのものを別に読んで、その name を使う。
  const knownFromSummary =
    summary !== undefined && (summary.rootPath !== undefined || folder.path === "");
  const rootLookup = useRootFolderName(
    folder.rootId,
    summary !== undefined && !knownFromSummary,
  );
  const rootName =
    summary === undefined
      ? undefined
      : knownFromSummary
        ? rootFolderName(summary)
        : rootLookup.status === "ready"
          ? rootLookup.name
          : undefined;
  // 登録フォルダの名前を読めなかったときは、その段を骨組みのまま残さずに省く。
  const rootNameFailed = !knownFromSummary && rootLookup.status === "failed";
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
  useEffect(() => scan.refresh(), [scan.refresh]);
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
      <FolderSearchResults folder={folder} videos={videos} zoom={zoom} backTo={backTo} />
    );
  } else if (noVideosAtAll) {
    body = (
      <EmptyState
        icon={FolderOpen}
        title="このフォルダにはまだ動画がありません"
        description={owner ? "取り込むと、ここに並びます。" : undefined}
        action={
          // 取り込みは所有者だけの操作である（ui-design.md「Guest degradation」）。
          owner ? (
            <Button variant="primary" onClick={scan.start} disabled={scan.running}>
              {scan.running ? "取り込み中…" : "取り込む"}
            </Button>
          ) : undefined
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
          listing.notFound || videos.notFound || listing.error !== null || rootNameFailed
            ? breadcrumbsFor(folder, undefined).filter((crumb) => crumb !== undefined)
            : breadcrumbsFor(folder, rootName)
        }
        suffix={searching && !videos.notFound ? "内を検索中" : undefined}
      />
      {/* 中身を押す直前（再生画面・子フォルダへ移る直前）の状態を控える。 */}
      <div onClick={saveSnapshot} className="flex flex-col gap-3">
        {body}
      </div>
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </>
  );
}
