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
import { useFolderListing, useRootFolders } from "../api/useFolderListing";
import { useVideos } from "../api/useVideos";
import { sortOptions } from "../library/LibraryToolbar";
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
 */
function useArrival() {
  const heading = useRef<HTMLHeadingElement | null>(null);
  useLayoutEffect(() => {
    window.scrollTo({ top: 0, behavior: "auto" });
    if (hasMounted) heading.current?.focus({ preventScroll: true });
    hasMounted = true;
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
  const sort =
    sortOptions.find((option) => option.value === searchParams.get("sort"))?.value ??
    preferences.sort;

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
  const heading = useArrival();
  const location = useLocation();
  const { sort, zoom, changeSort, changeZoom } = usePreferences();
  const scan = useScan();
  const listing = useFolderListing(folder);
  const videos = useVideos(sort, "", undefined, folder);

  const summary = listing.data?.folder;
  const children = listing.data?.folders ?? [];
  const rootName = summary === undefined ? undefined : rootDisplayName(summary.rootPath);
  const name =
    summary?.name ?? (folder.path === "" ? rootName : folder.path.split("/").at(-1));
  const backTo = `${location.pathname}${location.search}`;

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
      {body}
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
