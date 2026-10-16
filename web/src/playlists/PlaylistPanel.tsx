import { AlertTriangle, Check, ListVideo } from "lucide-react";
import { type ReactNode, useLayoutEffect, useRef } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import type { PlaylistItem } from "../api/playlists";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration, unplayableText } from "../lib/format";
import {
  CardNumber,
  cardListClass,
  isWideScreen,
  stickyHeadingClass,
  useRelatedScrub,
  VideoThumbnail as RelatedThumbnail,
} from "../player/RelatedVideos";
import { useHoverPreview } from "../player/useHoverPreview";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";
import { CollectionCountIcon, orderedItemCollection } from "../videoList/ItemRowCells";
import { itemTitle, playlistPath } from "./playlistItems";
import { itemStepPath, panelScrollTop } from "./playlistPlayback";
import type { PlaylistPanelState } from "./usePlaylistContext";

const headingId = "playlist-panel-heading";

/**
 * PlaylistPanel は動画ページの aside の先頭に置く、プレイリストの項目の並びである
 * （specs/044-playlists/ui-design.md「Playlist panel」）。項目は続けて再生と同じカードで、
 * 何本目かをサムネイルの左上に重ね、再生中の項目はリンクにせず枠と面で示す。広い画面では
 * 続けて再生・関連動画と同じ入れ物でスクロールし、狭い画面では一覧の高さを区切って中だけ
 * スクロールする。再生中のカードは `scrollTop` で見える位置に置く（ページと aside の列は動かさない）。
 * 並べ替えと外す操作はプレイリストの画面にあり、ここには置かない。
 */
export default function PlaylistPanel({
  playlistId,
  panel,
  name,
  position,
  count,
  currentItemId,
  backTo,
  autoplay,
  onRetry,
  scrollToCurrent = true,
}: {
  playlistId: number;
  panel: PlaylistPanelState;
  /** 見出しの名前。パネルの応答が無いときは再生の応答の名前。分からなければ undefined。 */
  name: string | undefined;
  /** 再生の応答の位置と数。分からないか位置が 0 なら、見出しの右に何も出さない。 */
  position: number;
  count: number;
  currentItemId: number | undefined;
  backTo: string;
  /** 行を押して移った先で再生を始めるか（矢印と同じ規則）。 */
  autoplay: boolean;
  onRetry: () => void;
  /**
   * 埋まったときに再生中のカードが見える位置へ動かすか。広い画面では右の列の並びの入れ物を、
   * 狭い画面ではパネル自身の一覧を動かす。続けて再生が下にあるときは、そちらが今のメンバーへ
   * 動かすので false にする。
   */
  scrollToCurrent?: boolean;
}) {
  const shownName = panel.kind === "ready" ? panel.playlist.name : name;
  return (
    <section
      aria-labelledby={shownName === undefined ? undefined : headingId}
      data-playlist-panel=""
      // 広い画面の右の列は画面の右端まで伸びるので、右の余白は区切りの側が持つ（下の並びと揃える）。
      className="flex shrink-0 flex-col"
    >
      {/* 広い画面では、続けて再生・関連動画と同じ入れ物でスクロールし、見出しはパネルが
          見えている間だけ上に留まる。 */}
      <div className={stickyHeadingClass}>
        {shownName === undefined ? (
          <Skeleton aria-hidden="true" className="h-5 w-1/2" />
        ) : (
          <h2
            id={headingId}
            className="flex min-w-0 items-baseline gap-1.5 text-sm font-semibold"
          >
            <ListVideo
              aria-hidden="true"
              className="size-4 shrink-0 self-center text-muted-foreground"
            />
            <Link
              to={playlistPath(playlistId)}
              state={{ from: backTo }}
              title={shownName}
              aria-label={t.player.playlist.line(shownName, 0, count)}
              className="min-w-0 truncate rounded-sm hover:underline"
            >
              {shownName}
            </Link>
          </h2>
        )}
        {shownName !== undefined && position > 0 && (
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {t.player.related.position(position, count)}
          </span>
        )}
      </div>
      {panel.kind === "loading" && (
        <ul
          aria-hidden="true"
          className={cn(
            cardListClass,
            "max-h-playlist-panel overflow-hidden lg:max-h-none",
          )}
        >
          {Array.from({ length: 3 }, (_, index) => (
            <li key={index} className="flex flex-col gap-2">
              <Skeleton className="aspect-video w-full" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-2/3" />
            </li>
          ))}
        </ul>
      )}
      {panel.kind === "failed" && (
        <div className="flex flex-col items-start gap-2">
          <p className="text-sm text-muted-foreground">{t.playlists.detail.loadFailed}</p>
          <Button variant="ghost" size="sm" onClick={onRetry}>
            {t.common.retry}
          </Button>
        </div>
      )}
      {panel.kind === "ready" && panel.playlist.items.length > 0 && (
        <PanelRows
          playlistId={playlistId}
          items={panel.playlist.items}
          currentItemId={currentItemId}
          backTo={backTo}
          autoplay={autoplay}
          scrollToCurrent={scrollToCurrent}
        />
      )}
    </section>
  );
}

function PanelRows({
  playlistId,
  items,
  currentItemId,
  backTo,
  autoplay,
  scrollToCurrent,
}: {
  playlistId: number;
  items: readonly PlaylistItem[];
  currentItemId: number | undefined;
  backTo: string;
  autoplay: boolean;
  scrollToCurrent: boolean;
}) {
  const listRef = useRef<HTMLUListElement | null>(null);
  const currentRef = useRef<HTMLLIElement | null>(null);
  const currentIndex = items.findIndex((item) => item.id === currentItemId);

  // 埋まったとき、再生中の項目が変わったとき、読み直した並びで再生中のカードの位置が変わった
  // ときに、再生中のカードを見える位置へ動かす。狭い画面ではパネル自身の一覧を、広い画面では
  // 右の列の並びの入れ物を動かす（そこでは一覧は高さを区切らず、入れ物が位置の基準になる）。
  // scrollIntoView はページと aside の列も動かすので使わない。
  useLayoutEffect(() => {
    const list = listRef.current;
    const current = currentRef.current;
    if (!scrollToCurrent || list === null || current === null) return;
    const previous = current.previousElementSibling;
    const top = panelScrollTop(
      previous instanceof HTMLElement
        ? { top: previous.offsetTop, height: previous.offsetHeight }
        : null,
    );
    const scroller = isWideScreen()
      ? list.closest<HTMLElement>("[data-related-scroller]")
      : null;
    if (scroller === null) {
      list.scrollTop = top;
      return;
    }
    // 上に留まる見出しの下から見えるよう、見出しの高さの分だけ戻す。
    const heading = list.parentElement?.firstElementChild;
    const covered = heading instanceof HTMLElement ? heading.offsetHeight : 0;
    scroller.scrollTop = Math.max(0, top - covered);
  }, [currentItemId, currentIndex, scrollToCurrent]);

  return (
    <ul
      ref={listRef}
      aria-labelledby={headingId}
      data-playlist-panel-list=""
      // 行の hover の面と輪郭が切れないよう、はみ出す分だけ内側に余白を取る。
      className={cn(
        cardListClass,
        "relative -mx-1.5 max-h-playlist-panel overflow-y-auto overscroll-contain scrollbar-on-hover px-1.5 py-1.5 lg:static lg:max-h-none lg:overflow-visible",
      )}
    >
      {items.map((item, index) =>
        item.id === currentItemId ? (
          <li key={item.id} ref={currentRef} aria-current="true">
            <div className="-m-1.5 flex flex-col gap-2 rounded-lg bg-accent p-1.5 ring-2 ring-primary ring-inset">
              <ItemThumbnail item={item} position={index + 1} />
              <span className="sr-only">{t.player.related.nowPlaying}</span>
              <ItemText item={item} current />
            </div>
          </li>
        ) : (
          <PanelRow
            key={item.id}
            playlistId={playlistId}
            item={item}
            position={index + 1}
            backTo={backTo}
            autoplay={autoplay}
          />
        ),
      )}
    </ul>
  );
}

function PanelRow({
  playlistId,
  item,
  position,
  backTo,
  autoplay,
}: {
  playlistId: number;
  item: PlaylistItem;
  position: number;
  backTo: string;
  autoplay: boolean;
}) {
  const to = itemStepPath(playlistId, item);
  const content = (
    <>
      <ItemThumbnail item={item} position={position} />
      <ItemText item={item} />
    </>
  );
  if (to === null) {
    return (
      <li>
        <div className="flex flex-col gap-2">{content}</div>
      </li>
    );
  }
  const state = { from: backTo, autoplay };
  const label = t.playlists.item.playFrom(itemTitle(item));
  if (item.kind === "video" && item.video !== undefined) {
    return (
      <VideoRow
        video={item.video}
        to={to}
        state={state}
        label={label}
        position={position}
        text={<ItemText item={item} />}
      />
    );
  }
  return (
    <li>
      <Link
        to={to}
        state={state}
        aria-label={label}
        className="-m-1.5 flex flex-col gap-2 rounded-lg p-1.5 transition-colors hover:bg-accent"
      >
        {content}
      </Link>
    </li>
  );
}

/** VideoRow は動画の項目のカードで、関連動画のカードと同じく hover のプレビューとスクラブを持つ。 */
function VideoRow({
  video,
  to,
  state,
  label,
  position,
  text,
}: {
  video: Video;
  to: string;
  state: { from: string; autoplay: boolean };
  label: string;
  position: number;
  text: ReactNode;
}) {
  const preview = useHoverPreview(video);
  const { scrub, release, setLink } = useRelatedScrub(video, preview);
  return (
    <li>
      <Link
        ref={setLink}
        to={to}
        state={state}
        aria-label={label}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={release}
        className="-m-1.5 flex flex-col gap-2 rounded-lg p-1.5 transition-colors hover:bg-accent"
      >
        <RelatedThumbnail
          video={video}
          preview={preview}
          scrub={scrub}
          marks={<CardNumber position={position} />}
        />
        {text}
      </Link>
    </li>
  );
}

/**
 * ItemThumbnail は再生中のカードと、グループとスマートフォルダのカードのサムネイルである
 * （プレビューは流さない）。何本目かを左上に重ねる。
 */
function ItemThumbnail({ item, position }: { item: PlaylistItem; position: number }) {
  if (item.kind === "video" && item.video !== undefined) {
    return (
      <RelatedThumbnail video={item.video} marks={<CardNumber position={position} />} />
    );
  }
  const group = orderedItemCollection(item);
  const thumbnailUrl = group?.previews[0]?.thumbnailUrl;
  const duration = formatDuration(group?.durationMs);
  const inProgress = group?.watchState === "inProgress" && group.videoCount > 0;
  const ratio = inProgress
    ? Math.min((group.watchedCount ?? 0) / group.videoCount, 1)
    : 0;
  return (
    <VideoThumbnail className="shrink-0">
      {thumbnailUrl !== undefined && <VideoThumbnailImage src={thumbnailUrl} />}
      {duration !== "" && (
        <VideoThumbnailDuration className="right-1 bottom-1 px-1 py-0 text-xs font-normal">
          {duration}
        </VideoThumbnailDuration>
      )}
      {inProgress && (
        <VideoThumbnailProgress
          value={Math.round(ratio * 100)}
          aria-label={t.library.group.watchedRatio}
        />
      )}
      <CardNumber position={position} />
    </VideoThumbnail>
  );
}

/**
 * ItemText は題名（グループとスマートフォルダは名前と本数）、視聴の印と再生できない理由である。
 * 動画の無いスマートフォルダの項目は名前を従の色にし、今の項目なら進み具合の欄に「0 videos」を
 * 出す（specs/052-smart-folders/ui-design.md「States」）。
 */
function ItemText({ item, current = false }: { item: PlaylistItem; current?: boolean }) {
  const title = itemTitle(item);
  const group = orderedItemCollection(item);
  const watched =
    group !== undefined
      ? group.watchState === "watched"
      : item.video?.progress?.completed === true;
  // 今の項目が動画の無いフォルダなら、進み具合の欄に「0 videos」を出す（ui-design.md「States」）。
  const groupProgress =
    group !== undefined && group.watchState === "inProgress"
      ? t.library.group.progress(group.watchedCount ?? 0, group.videoCount)
      : current && group?.videoCount === 0
        ? t.library.group.count(0)
        : null;
  const unplayable = item.video === undefined ? null : unplayableText(item.video);
  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex min-w-0 items-start gap-1">
        <span
          className={cn(
            "line-clamp-2 min-w-0 text-sm font-medium wrap-anywhere",
            watched || group?.videoCount === 0
              ? "text-muted-foreground"
              : "text-foreground",
          )}
        >
          {title}
        </span>
        {watched && (
          <>
            <Check
              className="mt-0.5 size-4 shrink-0 text-success"
              strokeWidth={2.5}
              aria-hidden="true"
            />
            <span className="sr-only">{t.player.related.watched}</span>
          </>
        )}
        {groupProgress !== null && (
          <span className="ml-auto shrink-0 pt-0.5 text-xs text-muted-foreground tabular-nums">
            {groupProgress}
          </span>
        )}
      </span>
      {group !== undefined && (
        <span className="flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <CollectionCountIcon smartFolder={item.kind === "smartFolder"} />
          {t.library.group.count(group.videoCount)}
        </span>
      )}
      {unplayable !== null && (
        <span className="flex items-center gap-1 text-xs text-warning">
          <AlertTriangle aria-hidden="true" className="size-3 shrink-0" />
          {unplayable}
        </span>
      )}
    </span>
  );
}
