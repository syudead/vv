import { AlertTriangle, Check, Folder, ListVideo } from "lucide-react";
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
import { itemTitle, playlistPath } from "./playlistItems";
import { itemStartVideoId, panelScrollTop, stepPath } from "./playlistPlayback";
import type { PlaylistPanelState } from "./usePlaylistContext";

const headingId = "playlist-panel-heading";

/**
 * PlaylistPanel は動画ページの aside の先頭に置く、プレイリストの項目の並びである
 * （specs/044-playlists/ui-design.md「Playlist panel」）。項目は続けて再生と同じカードで、
 * 何本目かをサムネイルの左上に重ね、再生中の項目はリンクにせず枠と面で示す。一覧は高さを抑えて中だけスクロールし、
 * 再生中の行を一覧自身の `scrollTop` で見える位置に置く（ページと aside の列は動かさない）。
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
}) {
  const shownName = panel.kind === "ready" ? panel.playlist.name : name;
  return (
    <section
      aria-labelledby={shownName === undefined ? undefined : headingId}
      data-playlist-panel=""
      // 広い画面の右の列は画面の右端まで伸びるので、右の余白は区切りの側が持つ（下の並びと揃える）。
      className="flex shrink-0 flex-col gap-3 lg:pr-3"
    >
      <div className="flex items-baseline justify-between gap-3">
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
          className={cn(cardListClass, "max-h-playlist-panel overflow-hidden")}
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
}: {
  playlistId: number;
  items: readonly PlaylistItem[];
  currentItemId: number | undefined;
  backTo: string;
  autoplay: boolean;
}) {
  const listRef = useRef<HTMLUListElement | null>(null);
  const currentRef = useRef<HTMLLIElement | null>(null);
  const currentIndex = items.findIndex((item) => item.id === currentItemId);

  // 埋まったとき、再生中の項目が変わったとき、読み直した並びで再生中の行の位置が変わったときに、
  // 一覧の中だけを動かす。scrollIntoView はページと aside の列も動かすので使わない。
  useLayoutEffect(() => {
    const list = listRef.current;
    const current = currentRef.current;
    if (list === null || current === null) return;
    const previous = current.previousElementSibling;
    list.scrollTop = panelScrollTop(
      previous instanceof HTMLElement
        ? { top: previous.offsetTop, height: previous.offsetHeight }
        : null,
    );
  }, [currentItemId, currentIndex]);

  return (
    <ul
      ref={listRef}
      aria-labelledby={headingId}
      data-playlist-panel-list=""
      // 行の hover の面と輪郭が切れないよう、はみ出す分だけ内側に余白を取る。
      className={cn(
        cardListClass,
        "relative -mx-1.5 max-h-playlist-panel overflow-y-auto overscroll-contain scrollbar-on-hover px-1.5 py-1.5",
      )}
    >
      {items.map((item, index) =>
        item.id === currentItemId ? (
          <li key={item.id} ref={currentRef} aria-current="true">
            <div className="-m-1.5 flex flex-col gap-2 rounded-lg bg-accent p-1.5 ring-2 ring-primary ring-inset">
              <ItemThumbnail item={item} position={index + 1} />
              <span className="sr-only">{t.player.related.nowPlaying}</span>
              <ItemText item={item} />
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
  const videoId = itemStartVideoId(item);
  const content = (
    <>
      <ItemThumbnail item={item} position={position} />
      <ItemText item={item} />
    </>
  );
  if (videoId === undefined) {
    return (
      <li>
        <div className="flex flex-col gap-2">{content}</div>
      </li>
    );
  }
  const to = stepPath(playlistId, videoId, item.id);
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
 * ItemThumbnail は再生中のカードとグループのカードのサムネイルである（プレビューは流さない）。
 * 何本目かを左上に重ねる。
 */
function ItemThumbnail({ item, position }: { item: PlaylistItem; position: number }) {
  if (item.kind === "video" && item.video !== undefined) {
    return (
      <RelatedThumbnail video={item.video} marks={<CardNumber position={position} />} />
    );
  }
  const group = item.group;
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

/** ItemText は題名（グループは名前と本数）、視聴の印と再生できない理由である。 */
function ItemText({ item }: { item: PlaylistItem }) {
  const title = itemTitle(item);
  const group = item.kind === "group" ? item.group : undefined;
  const watched =
    group !== undefined
      ? group.watchState === "watched"
      : item.video?.progress?.completed === true;
  const groupProgress =
    group !== undefined && group.watchState === "inProgress"
      ? t.library.group.progress(group.watchedCount ?? 0, group.videoCount)
      : null;
  const unplayable = item.video === undefined ? null : unplayableText(item.video);
  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex min-w-0 items-start gap-1">
        <span
          className={cn(
            "line-clamp-2 min-w-0 text-sm font-medium wrap-anywhere",
            watched ? "text-muted-foreground" : "text-foreground",
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
          <Folder aria-hidden="true" className="size-3 shrink-0" />
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
