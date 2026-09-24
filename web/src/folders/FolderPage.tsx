import { FolderOpen, FolderX } from "lucide-react";
import {
  type CSSProperties,
  type ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Link, useLocation } from "react-router";

import type { FolderRef, FolderScope, VideoSort } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { useFolderListing, useRootFolders } from "../api/useFolderListing";
import { useVideos } from "../api/useVideos";
import {
  clearConditions,
  hasConditions,
  type HistoryMode,
  type ListCriteria,
  newSeed,
} from "../library/listCriteria";
import { conditionLabels, summarize } from "../library/LibraryPage";
import { CardSkeleton, EmptyState, LoadFailed, NoMatches } from "../library/states";
import { useListCriteria } from "../library/useListCriteria";
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
  folderLocationLabel,
  parseFolderPathname,
  rootDisplayName,
  topLevelLocationLabel,
} from "./folderPath";

const cardWidth: Record<Zoom, string> = {
  0: "var(--spacing-card-0)",
  1: "var(--spacing-card-1)",
  2: "var(--spacing-card-2)",
  3: "var(--spacing-card-3)",
};

/**
 * ROOT_SEARCH_KEY は最上位の検索結果の控えの鍵である。実在するフォルダの鍵
 * （folderKey、`<id>\0<path>` の形）とは衝突しない文字列を選ぶ。
 */
const ROOT_SEARCH_KEY = "root-search";

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

/** rangeLabel は一致なしのチップに添える範囲の名前である（ui-design.md「No-match state」）。 */
function rangeLabel(kind: "subtree" | "direct", name: string): string {
  return kind === "subtree" ? `${name}とその中` : `${name}の直下`;
}

/**
 * useConditions は一覧の条件（検索語・視聴状態・再生可否・並べ替え・seed）を
 * URL から読み書きする口である。ライブラリと同じ `library/listCriteria`・
 * `useListCriteria` を使う（Plan の Structural Decisions 9）。端末に保存するのは
 * 並べ替えだけで、フォルダ画面もライブラリと同じ保存値を読み書きする。
 */
function useConditions() {
  const [preferences, setPreferences] = useState(readViewPreferences);
  const { criteria, apply } = useListCriteria(preferences.sort);
  const searchField = useRef<HTMLInputElement | null>(null);

  const savePreferences = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  const update = useCallback(
    (next: ListCriteria, mode: HistoryMode = "push") => apply(next, mode),
    [apply],
  );

  const changeSort = useCallback(
    (next: VideoSort) => {
      update({
        ...criteria,
        sort: next,
        seed: next === "random" ? newSeed(criteria.seed) : undefined,
      });
      savePreferences({ ...preferences, sort: next });
    },
    [criteria, preferences, savePreferences, update],
  );
  const shuffle = useCallback(
    () => update({ ...criteria, seed: newSeed(criteria.seed) }),
    [criteria, update],
  );
  const changeWatch = useCallback(
    (next: ListCriteria["watch"]) => update({ ...criteria, watch: next }),
    [criteria, update],
  );
  const changePlayable = useCallback(
    (next: boolean) => update({ ...criteria, playable: next }),
    [criteria, update],
  );
  const commitQuery = useCallback(
    (next: string, mode: HistoryMode) => update({ ...criteria, query: next }, mode),
    [criteria, update],
  );
  const clearAll = useCallback(
    () => update(clearConditions(criteria)),
    [criteria, update],
  );
  // 一致なしの「条件を解除」は自分自身が消えるので、フォーカスを空になった検索欄へ移す。
  const clearFromNoMatches = useCallback(() => {
    clearAll();
    searchField.current?.focus();
  }, [clearAll]);
  const changeZoom = useCallback(
    (next: Zoom) => savePreferences({ ...preferences, zoom: next }),
    [preferences, savePreferences],
  );

  return {
    criteria,
    zoom: preferences.zoom,
    searchField,
    changeSort,
    shuffle,
    changeWatch,
    changePlayable,
    commitQuery,
    clearAll,
    clearFromNoMatches,
    changeZoom,
  };
}

/**
 * RootSearchResults は最上位（`/folders`）で検索語があるときの検索結果である。
 * ライブラリ全体を `listVideos` で検索し、置き場所は登録フォルダの表示名から
 * 始める（ui-design.md「Search results」）。
 */
function RootSearchResults({
  criteria,
  rootNames,
  zoom,
  onClearNoMatches,
}: {
  criteria: ListCriteria;
  rootNames: Map<number, string>;
  zoom: Zoom;
  onClearNoMatches: () => void;
}) {
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const [restored] = useState(() =>
    takeListSnapshot({ ...criteria, folder: ROOT_SEARCH_KEY }),
  );
  const {
    items,
    total,
    cursor,
    hasMore,
    loading,
    loadingMore,
    error,
    loadMore,
    retryLoadMore,
    reload,
  } = useVideos(criteria, restored);

  const scan = useScan();
  const knownScanId = useRef(restored?.scanId);
  useEffect(() => scan.refresh(), [scan.refresh]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    clearListSnapshot();
    reload();
  }, [reload, scan.finished]);

  const saveSnapshot = useCallback(() => {
    saveListSnapshot(
      { ...criteria, folder: ROOT_SEARCH_KEY },
      {
        items,
        total,
        cursor,
        hasMore,
        scrollY: window.scrollY,
        scanId: knownScanId.current,
      },
    );
  }, [criteria, cursor, hasMore, items, total]);

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
    if (top === undefined || items.length === 0) return;
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
    requestAnimationFrame(() => window.scrollTo({ top, behavior: "auto" }));
  }, [items.length]);

  const sentinel = useRef<HTMLDivElement | null>(null);
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

  const noMatch = !loading && error === null && items.length === 0;
  const summaryText = loading
    ? "読み込み中…"
    : `「${criteria.query}」 ${summarize(items, total)}`;

  return (
    <div onClick={saveSnapshot} className="flex flex-col gap-3">
      <h2 className="sr-only">検索結果</h2>
      {noMatch ? (
        <NoMatches
          conditions={[...conditionLabels(criteria), "すべてのフォルダ"]}
          onClear={onClearNoMatches}
        />
      ) : (
        <>
          <p
            role="status"
            aria-live="polite"
            className="text-center text-xs text-fg-muted tabular-nums"
          >
            {summaryText}
          </p>
          {error !== null && items.length === 0 ? (
            <LoadFailed reason={error} onRetry={reload} />
          ) : (
            <Grid zoom={zoom}>
              {loading ? (
                <CardSkeleton count={12} />
              ) : (
                items.map((video) => (
                  <VideoCard
                    key={video.id}
                    video={video}
                    backTo={backTo}
                    selected={false}
                    selectionMode={false}
                    location={
                      video.folder === undefined
                        ? undefined
                        : topLevelLocationLabel(
                            video.folder,
                            rootNames.get(video.folder.rootId),
                          )
                    }
                  />
                ))
              )}
              {loadingMore && <CardSkeleton count={6} />}
            </Grid>
          )}
          {error !== null && items.length > 0 && (
            <div className="flex items-center justify-center gap-2 text-sm text-danger">
              <p>続きを取得できません: {error}</p>
              <Button size="sm" onClick={retryLoadMore}>
                再試行
              </Button>
            </div>
          )}
        </>
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </div>
  );
}

/** RootView はフォルダ画面の最上位で、登録済みメディアフォルダを並べる。 */
function RootView() {
  const heading = useArrival();
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
    clearFromNoMatches,
    changeZoom,
  } = useConditions();
  const roots = useRootFolders();
  const folders = roots.data?.folders ?? [];
  const rootNames = useMemo(
    () => new Map(folders.map((folder) => [folder.rootId, folder.name])),
    [folders],
  );
  const searching = criteria.query !== "";

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
        <FolderToolbar
          query={criteria.query}
          onQueryCommit={commitQuery}
          searchRef={searchField}
          searchLabel="すべてのフォルダの動画を検索"
          searchPlaceholder="すべてのフォルダを検索"
          sort={criteria.sort}
          onSortChange={changeSort}
          onShuffle={shuffle}
          watch={criteria.watch}
          onWatchChange={changeWatch}
          playable={criteria.playable}
          onPlayableChange={changePlayable}
          canClear={hasConditions(criteria)}
          onClear={clearAll}
          disabled={!searching}
          zoom={zoom}
          onZoomChange={changeZoom}
        />
      </TopBarPortal>
      <Breadcrumbs
        crumbs={
          searching ? [{ label: "すべてのフォルダを検索中" }] : [{ label: "フォルダ" }]
        }
      />

      {searching ? (
        <RootSearchResults
          criteria={criteria}
          rootNames={rootNames}
          zoom={zoom}
          onClearNoMatches={clearFromNoMatches}
        />
      ) : roots.error !== null ? (
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

/** FolderView はフォルダ1件の中身（直下の子フォルダと動画、または検索結果）を並べる。 */
function FolderView({ folder }: { folder: FolderRef }) {
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
    clearFromNoMatches,
    changeZoom,
  } = useConditions();
  const scan = useScan();
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
  if (listing.notFound) {
    body = <FolderNotFound />;
  } else if (listing.error !== null) {
    body = <LoadFailed reason={listing.error} onRetry={listing.reload} />;
  } else if (searching) {
    // 検索結果（ui-design.md「Search results」）。子フォルダの一群と「動画 N」の
    // 見出しを出さず、ライブラリと同じ要約行と1つの格子にする。
    const noMatch = !videos.loading && videos.error === null && videos.items.length === 0;
    body = (
      <>
        <h2 className="sr-only">検索結果</h2>
        {noMatch ? (
          <NoMatches
            conditions={[
              ...conditionLabels(criteria),
              rangeLabel("subtree", name ?? "フォルダ"),
            ]}
            note={undefined}
            onClear={clearFromNoMatches}
          />
        ) : (
          <>
            <p
              role="status"
              aria-live="polite"
              className="text-center text-xs text-fg-muted tabular-nums"
            >
              {videos.loading
                ? "読み込み中…"
                : `「${criteria.query}」 ${summarize(videos.items, videos.total)}`}
            </p>
            {videos.error !== null && videos.items.length === 0 ? (
              <LoadFailed reason={videos.error} onRetry={videos.reload} />
            ) : (
              <Grid zoom={zoom}>
                {videos.loading ? (
                  <CardSkeleton count={12} />
                ) : (
                  videos.items.map((video) => (
                    <VideoCard
                      key={video.id}
                      video={video}
                      backTo={backTo}
                      selected={false}
                      selectionMode={false}
                      location={
                        video.folder === undefined
                          ? undefined
                          : folderLocationLabel(folder, video.folder)
                      }
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
          </>
        )}
      </>
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
    const showFolders = listing.loading || children.length > 0;
    const filterOnly = hasConditions(criteria);
    const filterOnlyNoMatch =
      filterOnly && !videos.loading && videos.error === null && videos.items.length === 0;
    const showVideos =
      videos.loading ||
      videos.items.length > 0 ||
      videos.error !== null ||
      filterOnlyNoMatch;
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
          <div className={showFolders ? "mt-3" : undefined}>
            {filterOnlyNoMatch ? (
              <>
                {/* 件数の変化は、視覚的に隠した polite の状態の行で知らせる（子フォルダの
                    上の「動画 N」は出さないので、見える要約行は無い）。 */}
                <p role="status" aria-live="polite" className="sr-only">
                  直下の動画 {videos.total.toLocaleString("ja-JP")} 件
                </p>
                <NoMatches
                  conditions={[
                    ...conditionLabels(criteria),
                    rangeLabel("direct", name ?? "フォルダ"),
                  ]}
                  note="中のフォルダも探すには、検索語を入れてください。"
                  onClear={clearFromNoMatches}
                />
              </>
            ) : (
              <Section title="動画" count={videos.loading ? undefined : videos.total}>
                {filterOnly && !videos.loading && (
                  <p role="status" aria-live="polite" className="sr-only">
                    直下の動画 {videos.total.toLocaleString("ja-JP")} 件
                  </p>
                )}
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
          </div>
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
          listing.notFound || listing.error !== null
            ? breadcrumbsFor(folder, undefined).filter((crumb) => crumb !== undefined)
            : breadcrumbsFor(folder, rootName)
        }
        suffix={searching ? "内を検索中" : undefined}
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
