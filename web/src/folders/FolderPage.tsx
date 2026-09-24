import { FolderOpen, FolderX } from "lucide-react";
import {
  type CSSProperties,
  type ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { Link, useLocation, useSearchParams } from "react-router";

import type { FolderRef, VideoSort } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { useFolderListing, useRootFolders } from "../api/useFolderListing";
import { useVideos } from "../api/useVideos";
import { isVideoSort } from "../library/listCriteria";
import { CardSkeleton, EmptyState, LoadFailed } from "../library/states";
import VideoCard from "../library/VideoCard";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useScan } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import Button, { buttonClassName } from "../ui/Button";
import Breadcrumbs from "./Breadcrumbs";
import FolderCard, { FolderCardSkeleton } from "./FolderCard";
import FolderToolbar from "./FolderToolbar";
import {
  breadcrumbsFor,
  FOLDERS_ROOT,
  folderKey,
  parseFolderPathname,
  rootDisplayName,
} from "./folderPath";

const cardWidth: Record<Zoom, string> = {
  0: "var(--spacing-card-0)",
  1: "var(--spacing-card-1)",
  2: "var(--spacing-card-2)",
  3: "var(--spacing-card-3)",
};

/** hasMounted はこのタブで最初の表示が済んだかを覚える。最初の表示ではフォーカスを動かさない。 */
let hasMounted = false;

/**
 * useArrival は別のフォルダへ移ったとき、見出しへフォーカスを移して先頭へ
 * スクロールする。読み上げソフトの利用者が移動と現在地を知れるようにする。
 * 控えから戻ってきたとき（restoring）は、元の位置へ戻すのでどちらもしない。
 */
function useArrival(restoring = false) {
  const heading = useRef<HTMLHeadingElement | null>(null);
  useLayoutEffect(() => {
    if (!restoring) {
      window.scrollTo({ top: 0, behavior: "auto" });
      if (hasMounted) heading.current?.focus({ preventScroll: true });
    }
    hasMounted = true;
    // 到着はフォルダごとに1回だけ扱う（FolderView は key でフォルダごとに作り直す）。
  }, []);
  return heading;
}

/** Grid はライブラリと同じ格子（同じ幅・同じ間隔）である。 */
function Grid({ zoom, children }: { zoom: Zoom; children: ReactNode }) {
  return (
    <div
      className="flex flex-wrap justify-center gap-2.5 [&>*]:w-[min(var(--card),100%)]"
      style={{ "--card": cardWidth[zoom] } as CSSProperties}
    >
      {children}
    </div>
  );
}

/** Section は見出し付きの一群である。見出しの件数も読み上げる。 */
function Section({
  title,
  count,
  className,
  children,
}: {
  title: string;
  count?: number;
  className?: string;
  children: ReactNode;
}) {
  return (
    <section className={"flex flex-col gap-2 " + (className ?? "")}>
      <h2 className="px-0.5 text-xs font-semibold text-fg-muted">
        {title}
        {count !== undefined && (
          // 読み上げで名前と件数が続けて読まれないよう、余白ではなく空白で区切る。
          <span className="tabular-nums"> {count.toLocaleString("ja-JP")}</span>
        )}
      </h2>
      {children}
    </section>
  );
}

function FolderNotFound() {
  return (
    <EmptyState
      icon={FolderX}
      title="このフォルダは見つかりません"
      description="登録が外れたか、中の動画が無くなりました。"
      action={
        <Link to={FOLDERS_ROOT} className={buttonClassName()}>
          フォルダの一覧へ
        </Link>
      }
    />
  );
}

/** usePreferences は並び順と表示倍率を、ライブラリと同じ保存値で扱う（要件 13）。 */
function usePreferences() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [preferences, setPreferences] = useState(readViewPreferences);
  const requested = searchParams.get("sort");
  const sort = isVideoSort(requested) ? requested : preferences.sort;

  const save = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  const changeSort = useCallback(
    (next: VideoSort) => {
      setSearchParams(
        (current) => {
          const params = new URLSearchParams(current);
          params.set("sort", next);
          return params;
        },
        { replace: true },
      );
      save({ ...preferences, sort: next });
    },
    [preferences, save, setSearchParams],
  );
  const changeZoom = useCallback(
    (next: Zoom) => save({ ...preferences, zoom: next }),
    [preferences, save],
  );
  return { sort, zoom: preferences.zoom, changeSort, changeZoom };
}

/** RootView はフォルダ画面の最上位で、登録済みメディアフォルダを並べる。 */
function RootView() {
  const heading = useArrival();
  const { zoom, changeSort, changeZoom } = usePreferences();
  const roots = useRootFolders();
  const folders = roots.data?.folders ?? [];

  // 取り込みが終わったら登録フォルダの集計を読み直す（Edge Case「取り込み中」）。
  const scan = useScan();
  const knownScanId = useRef(scan.finished?.id);
  const { reload } = roots;
  useEffect(() => scan.refresh(), [scan.refresh]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    reload();
  }, [reload, scan.finished]);

  return (
    <>
      <h1 ref={heading} tabIndex={-1} className="sr-only">
        フォルダ
      </h1>
      <TopBarPortal>
        <FolderToolbar onSortChange={changeSort} zoom={zoom} onZoomChange={changeZoom} />
      </TopBarPortal>
      <Breadcrumbs crumbs={[{ label: "フォルダ" }]} />

      {roots.error !== null ? (
        <LoadFailed reason={roots.error} onRetry={roots.reload} />
      ) : !roots.loading && folders.length === 0 ? (
        <EmptyState
          icon={FolderOpen}
          title="メディアフォルダが登録されていません"
          description="設定でメディアフォルダを登録して取り込むと、ここに並びます。"
          action={
            <Link to="/settings" className={buttonClassName("primary")}>
              設定を開く
            </Link>
          }
        />
      ) : (
        <Section
          title="メディアフォルダ"
          count={roots.loading ? undefined : folders.length}
        >
          <Grid zoom={zoom}>
            {roots.loading ? (
              <FolderCardSkeleton count={6} />
            ) : (
              folders.map((folder) => (
                <FolderCard key={folder.rootId} folder={folder} showPath />
              ))
            )}
          </Grid>
        </Section>
      )}
    </>
  );
}

/** FolderView はフォルダ1件の中身（直下の子フォルダと動画）を並べる。 */
function FolderView({ folder }: { folder: FolderRef }) {
  const location = useLocation();
  const { sort, zoom, changeSort, changeZoom } = usePreferences();
  const scan = useScan();

  // --- 再生画面から戻ったときは控えから復元する（要件 10） ---
  const key = folderKey(folder);
  const [restored] = useState(() => {
    const held = takeListSnapshot({ query: "", sort, folder: key });
    return held?.folderListing === undefined ? undefined : held;
  });
  const heading = useArrival(restored !== undefined);
  const listing = useFolderListing(folder, restored?.folderListing);
  const videos = useVideos({ sort }, restored, folder);

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
      { query: "", sort, folder: key },
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
    key,
    listing.data,
    sort,
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

  const empty =
    !listing.loading &&
    !videos.loading &&
    summary !== undefined &&
    children.length === 0 &&
    videos.items.length === 0 &&
    videos.error === null;

  let body: ReactNode;
  if (listing.notFound) {
    body = <FolderNotFound />;
  } else if (listing.error !== null) {
    body = <LoadFailed reason={listing.error} onRetry={listing.reload} />;
  } else if (empty) {
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
    const showFolders = listing.loading || children.length > 0;
    const showVideos = videos.loading || videos.items.length > 0 || videos.error !== null;
    body = (
      <>
        {showFolders && (
          <Section title="フォルダ" count={listing.loading ? undefined : children.length}>
            <Grid zoom={zoom}>
              {listing.loading ? (
                <FolderCardSkeleton count={3} />
              ) : (
                children.map((child) => (
                  <FolderCard key={child.path} folder={child} showPath={false} />
                ))
              )}
            </Grid>
          </Section>
        )}
        {showVideos && (
          <Section
            title="動画"
            count={videos.loading ? undefined : videos.total}
            className={showFolders ? "mt-3" : undefined}
          >
            {videos.error !== null && videos.items.length === 0 ? (
              <LoadFailed reason={videos.error} onRetry={videos.reload} />
            ) : (
              <Grid zoom={zoom}>
                {videos.loading ? (
                  <CardSkeleton count={6} />
                ) : (
                  videos.items.map((video) => (
                    <VideoCard
                      key={video.id}
                      video={video}
                      backTo={backTo}
                      selected={false}
                      selectionMode={false}
                    />
                  ))
                )}
                {videos.loadingMore && <CardSkeleton count={6} />}
              </Grid>
            )}
            {videos.error !== null && videos.items.length > 0 && (
              <div className="flex items-center justify-center gap-2 text-sm text-danger">
                <p>続きを取得できません: {videos.error}</p>
                <Button size="sm" onClick={videos.retryLoadMore}>
                  再試行
                </Button>
              </div>
            )}
          </Section>
        )}
      </>
    );
  }

  return (
    <>
      <h1 ref={heading} tabIndex={-1} className="sr-only">
        {name ?? "フォルダ"}
      </h1>
      <TopBarPortal>
        <FolderToolbar
          sort={sort}
          onSortChange={changeSort}
          zoom={zoom}
          onZoomChange={changeZoom}
        />
      </TopBarPortal>
      <Breadcrumbs
        crumbs={
          // 見つからなかったフォルダでは登録フォルダの名前が分からないので、その段を出さない。
          listing.notFound || listing.error !== null
            ? breadcrumbsFor(folder, undefined).filter((crumb) => crumb !== undefined)
            : breadcrumbsFor(folder, rootName)
        }
      />
      {/* 中身を押す直前（再生画面・子フォルダへ移る直前）の状態を控える。 */}
      <div onClick={saveSnapshot} className="flex flex-col gap-3">
        {body}
      </div>
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </>
  );
}

/**
 * FolderPage はフォルダ画面である。URL（`/folders`・`/folders/{rootId}/{段}…`）
 * から場所を読み、フォルダごとに中身を作り直す（key で状態を分ける）。
 */
export default function FolderPage() {
  const location = useLocation();
  const target = parseFolderPathname(location.pathname);

  return (
    <div className="flex w-full flex-col gap-3 px-3 pt-3 pb-24 sm:px-4">
      {target.kind === "root" ? (
        <RootView />
      ) : target.kind === "folder" ? (
        <FolderView key={folderKey(target.folder)} folder={target.folder} />
      ) : (
        <>
          <h1 className="sr-only">フォルダ</h1>
          <Breadcrumbs
            crumbs={[{ label: "フォルダ", to: FOLDERS_ROOT }, { label: "…" }]}
          />
          <FolderNotFound />
        </>
      )}
    </div>
  );
}
