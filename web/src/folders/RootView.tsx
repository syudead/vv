import { FolderOpen } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";

import { takeListSnapshot } from "../api/listSnapshot";
import { useRootFolders } from "../api/useFolderListing";
import { useAudience } from "../auth/audience";
import { useScan } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import { buttonClassName } from "../ui/Button";
import { Grid } from "../videoList/Grid";
import { hasConditions } from "../videoList/listCriteria";
import { EmptyState, GuestEmpty, LoadFailed } from "../videoList/states";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import Breadcrumbs from "./Breadcrumbs";
import FolderCard, { FolderCardSkeleton } from "./FolderCard";
import FolderToolbar from "./FolderToolbar";
import { type RootDisplay, rootFolderName } from "./folderPath";
import { Section } from "./layout";
import RootSearchResults, { ROOT_SEARCH_KEY } from "./RootSearchResults";
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
  const heading = useArrival(restored !== undefined);
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
  const scan = useScan();
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

      <div ref={listRef} className="flex flex-col gap-3">
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
            <GuestEmpty />
          )
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
                  <FolderCard key={folder.rootId} folder={folder} showPath={owner} />
                ))
              )}
            </Grid>
          </Section>
        )}
      </div>
    </>
  );
}
