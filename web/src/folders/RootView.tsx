import { FolderOpen } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";

import { takeListSnapshot } from "../api/listSnapshot";
import { useRootFolders } from "../api/useFolderListing";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { useScanControls } from "../shell/ScanProvider";
import { CardGrid } from "../ui/patterns/card-grid";
import { ListPage } from "../ui/patterns/list-page";
import { Button } from "../ui/shadcn/button";
import { hasConditions } from "../videoList/listCriteria";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import Breadcrumbs from "./Breadcrumbs";
import FolderCard from "./FolderCard";
import TopBarPortal from "../shell/TopBarPortal";
import FolderToolbar from "./FolderToolbar";
import { type RootDisplay, rootFolderName } from "./folderPath";
import { Section } from "./layout";
import RootSearchResults, { ROOT_SEARCH_KEY } from "./RootSearchResults";
import { CardsLoading, FolderEmpty, GuestEmpty, LoadFailed } from "./states";
import { useArrival } from "./useArrival";
import { useConditions } from "./useConditions";

/** RootView はフォルダ画面の最上位で、登録済みメディアフォルダを並べる。 */
export default function RootView() {
  const {
    criteria,
    zoom,
    searchField,
    changeSort,
    shuffle,
    changeWatch,
    changePlayable,
    changeFavorite,
    commitQuery,
    clearAll,
    changeZoom: saveZoom,
  } = useConditions();
  // ゲストには登録フォルダの絶対パスも、設定への入口も出さない
  // （specs/016-single-account-auth/ui-design.md「Guest degradation」）。
  const owner = useAudience() === "owner";
  // 倍率を変えても読んでいた位置を保つ（ライブラリと同じ）。
  const { listRef, capture } = useZoomAnchor(zoom);
  const changeZoom = (next: typeof zoom) => {
    capture();
    saveZoom(next);
  };
  // 再生画面から検索結果へ戻ったときは控えから復元する（FolderView と同じ扱い）。
  // 検索していなければ動画の一覧を持たないので、控えを探さない。
  const [restored] = useState(() =>
    criteria.query === ""
      ? undefined
      : takeListSnapshot({ ...criteria, folder: ROOT_SEARCH_KEY }),
  );
  const heading = useArrival<HTMLHeadingElement>(restored !== undefined);
  const roots = useRootFolders();
  const folders = useMemo(() => roots.data?.folders ?? [], [roots.data?.folders]);
  // パンくずと同じ規則（rootFolderName）で表示名を作る。絶対パスがあれば
  // rootDisplayName で、ゲストの応答のように無ければサーバーの FolderSummary.name を使う。
  const rootNames = useMemo(
    () =>
      new Map<number, RootDisplay>(
        folders.map((folder) => [
          folder.rootId,
          { name: rootFolderName(folder), rootPath: folder.rootPath },
        ]),
      ),
    [folders],
  );
  const searching = criteria.query !== "";

  // 取り込みが終わったら登録フォルダの集計を読み直す（Edge Case「取り込み中」）。
  const scan = useScanControls();
  const { refresh: refreshScan } = scan;
  const knownScanId = useRef(scan.finished?.id);
  const { reload } = roots;
  useEffect(() => refreshScan(), [refreshScan]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    reload();
  }, [reload, scan.finished]);

  return (
    <ListPage
      header={
        <>
          <Breadcrumbs
            crumbs={
              searching
                ? [{ label: t.folders.searchingAll }]
                : [{ label: t.folders.title }]
            }
          />
          {/* 現在地はパンくずが見せるので、題は読み上げだけに置く。 */}
          <h1 ref={heading} tabIndex={-1} className="sr-only">
            {t.folders.title}
          </h1>
        </>
      }
      toolbar={
        <TopBarPortal>
          <FolderToolbar
            query={criteria.query}
            onQueryCommit={commitQuery}
            searchRef={searchField}
            searchLabel={t.folders.searchAll}
            searchPlaceholder={t.folders.searchAllPlaceholder}
            sort={criteria.sort}
            onSortChange={changeSort}
            onShuffle={shuffle}
            watch={criteria.watch}
            onWatchChange={changeWatch}
            playable={criteria.playable}
            onPlayableChange={changePlayable}
            favorite={criteria.favorite}
            onFavoriteChange={changeFavorite}
            canClear={hasConditions(criteria)}
            onClear={clearAll}
            disabled={!searching}
            zoom={zoom}
            onZoomChange={changeZoom}
          />
        </TopBarPortal>
      }
    >
      {/* 倍率を変える前の位置の目印を探す入れ物。間隔は骨格が持つので箱を作らない。 */}
      <div ref={listRef} className="contents">
        {searching ? (
          <RootSearchResults
            criteria={criteria}
            rootNames={rootNames}
            roots={roots}
            zoom={zoom}
            restored={restored}
          />
        ) : roots.error !== null ? (
          <LoadFailed reason={roots.error} onRetry={roots.reload} />
        ) : !roots.loading && folders.length === 0 ? (
          owner ? (
            <FolderEmpty
              icon={<FolderOpen aria-hidden="true" />}
              title={t.folders.noMediaFolders.title}
              description={t.folders.noMediaFolders.description}
              action={
                <Button asChild size="sm">
                  <Link to="/settings">{t.folders.noMediaFolders.openSettings}</Link>
                </Button>
              }
            />
          ) : (
            <GuestEmpty />
          )
        ) : (
          <Section
            title={t.folders.mediaFolders}
            count={roots.loading ? undefined : folders.length}
          >
            {roots.loading ? (
              <CardsLoading zoom={zoom} count={6} />
            ) : (
              <CardGrid size={zoom}>
                {folders.map((folder) => (
                  <FolderCard key={folder.rootId} folder={folder} showPath={owner} />
                ))}
              </CardGrid>
            )}
          </Section>
        )}
      </div>
    </ListPage>
  );
}
