import { Check, Folder } from "lucide-react";
import { memo, type MouseEvent, type ReactNode } from "react";
import { Link } from "react-router";

import type { LibraryGroup, LibraryGroupCard } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { useAudience } from "../auth/audience";
import { formatRelative, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatBytes, formatDuration } from "../lib/format";
import AddToGroupButton from "../groups/AddToGroupButton";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { Checkbox } from "../ui/shadcn/checkbox";
import { TableCell, TableRow } from "../ui/shadcn/table";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailMark,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";
import { groupPagePath } from "../groups/groupPaths";
import { type CardDragBinding, dragRowProps, RowDropMarks } from "../videoList/cardDrag";
import { cardLinkState } from "../videoList/cardLink";
import FolderArt from "../videoList/FolderArt";

/**
 * GroupCardProps はライブラリのグループのカードと行の props である
 * （specs/017-folder-groups/ui-design.md「Group card」）。グループのカードはフォルダ画面には
 * 出ないので `web/src/library/` に置く。グループ G はライブラリでは LibraryGroup、ホームの棚では
 * カードの値だけの LibraryGroupCard である（specs/057-home/research.md R-12）。
 */
export interface GroupCardProps<G extends LibraryGroupCard = LibraryGroup> {
  group: G;
  /** 遷移元の一覧 URL。開いた先の「戻る」がここへ帰る。 */
  backTo: string;
  /**
   * カードと行の名前が開く場所。省くとそのグループのページ（`/groups/{id}`、道筋なし）で、
   * ライブラリはこれを使う。グループのページは道筋を 1 段伸ばした場所を渡す
   * （specs/048-groups/ui-design.md「Group card and row」「Body」）。
   */
  to?: string;
  /** 全メンバーが選択に入っているときだけ true（ui-design.md「Pressing and selection」）。 */
  selected: boolean;
  selectionMode: boolean;
  /**
   * 全メンバー（`videoIds`）を選択に入れる・外す。呼び出し元はグループとしても覚える
   * （specs/035-favorites/research.md R-7）。選択を持たない見る人（ゲスト）では
   * 省き、チェックを描かない。
   */
  onSelect?: (group: G, selected: boolean) => void;
  /** 一覧のホバープレビューの調整（usePreviewCoordination）。格子のフォルダの絵柄が加わる。 */
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  onPreviewReset?: () => void;
  /** 題名の下のタグの行（格子表示だけ）。VideoCard の tagsRow と同じく関数で受ける。 */
  tagsRow?: (group: G) => ReactNode;
  /**
   * 名前の位置に出す欄。「Make a group」で作ったばかりのグループだけが持つ
   * （specs/050-drag-organize/ui-design.md「The new group's name」）。欄の間、名前はリンクの外に出る。
   */
  nameField?: ReactNode;
  /** リストの行をドラッグの層に加える値（useCardDrag）。所有者のライブラリのリスト表示だけが渡す。 */
  drag?: CardDragBinding;
  /** 格子のカードが開く再生画面で再生を始める（VideoCard の autoplay と同じ）。 */
  autoplay?: boolean;
  /** 格子のカードの右上に、所有者のお気に入りの付け外しだけを置く（VideoCard と同じ）。 */
  favoriteOnly?: boolean;
}

/** CollectionValues はグループとスマートフォルダのカードが共有する、視聴と長さの値である。 */
export type CollectionValues = Pick<
  LibraryGroup,
  "videoCount" | "watchedCount" | "watchState" | "durationMs"
>;

/**
 * useCollectionFacts は見る人に見せるグループかスマートフォルダの視聴の値である。ゲストでは
 * 視聴状態を出さない（ui-design.md「Screen boundary」の「ゲスト」）。格子のカードは見終えた
 * 本数を数字では出さず、見ている途中の帯の長さと読み上げ名にだけ使う。
 */
export function useCollectionFacts(values: CollectionValues) {
  const owner = useAudience() === "owner";
  const watchedCount = owner ? (values.watchedCount ?? 0) : 0;
  const state = owner ? (values.watchState ?? "unwatched") : "unwatched";
  const ratio =
    state === "inProgress" && values.videoCount > 0
      ? Math.min(watchedCount / values.videoCount, 1)
      : null;
  return {
    watchedCount,
    state,
    ratio,
    duration: formatDuration(values.durationMs),
    countText: groupCountText(values.videoCount),
  };
}

/** useGroupFacts はグループの視聴の値と、リンクの読み上げ名である。 */
function useGroupFacts(group: LibraryGroupCard) {
  const facts = useCollectionFacts(group);
  return {
    ...facts,
    label: groupLinkLabel(group.name, group.videoCount, facts.watchedCount),
  };
}

/** groupCountText はカードの本数の文字（「12 videos」）である。 */
export function groupCountText(videoCount: number): UiText {
  return t.library.group.count(videoCount);
}

/** groupLinkLabel はカードのリンクの読み上げ名である（ui-design.md「Accessibility」）。 */
export function groupLinkLabel(
  name: string,
  videoCount: number,
  watchedCount: number,
): UiText {
  return t.library.group.label(name, videoCount, watchedCount);
}

/**
 * GroupFavorite はグループのカード・行のお気に入りの付け外しである（所有者だけ。
 * specs/035-favorites/ui-design.md「Card」）。グループの id を送り、メンバーには付けない
 * （specs/048-groups/research.md R-10）。
 */
function GroupFavorite({
  group,
  variant,
}: {
  group: LibraryGroupCard;
  variant: "card" | "row";
}) {
  const favorite = group.favorite === true;
  return (
    <FavoriteToggle
      favorite={favorite}
      label={t.library.group.favorite(group.name)}
      onToggle={() => updateFavorites([], [group.id], !favorite)}
      variant={variant}
    />
  );
}

/**
 * GroupAddToPlaylist はグループのカード・行の「Add to playlist」である（所有者だけ。
 * specs/044-playlists/ui-design.md「Add to playlist」）。グループの id を送る
 * （specs/048-groups/research.md R-10）。
 */
function GroupAddToPlaylist({
  group,
  variant,
}: {
  group: LibraryGroupCard;
  variant: "card" | "row";
}) {
  return (
    <AddToPlaylistButton
      variant={variant}
      label={t.library.group.addToPlaylist(group.name)}
      groupIds={[group.id]}
    />
  );
}

/**
 * GroupAddToGroup はグループのカード・行の「Add to group」である（所有者だけ。
 * specs/048-groups/ui-design.md「Group card and row」）。グループの id を送る。
 */
function GroupAddToGroup({
  group,
  variant,
}: {
  group: LibraryGroupCard;
  variant: "card" | "row";
}) {
  return (
    <AddToGroupButton
      variant={variant}
      label={t.library.group.addToGroup(group.name)}
      groupIds={[group.id]}
    />
  );
}

/**
 * CollectionThumbnail はグループとスマートフォルダのカードのサムネイルの枠である。フォルダの
 * 絵柄（FolderArt）に、動画のカードの長さと同じ面で右下に `face`（本数と長さ）を重ね、見ている
 * 途中なら帯を出す。面と帯は前に出たサムネイルより上に置き、絵柄の下見の操作を妨げない。
 */
export function CollectionThumbnail({
  previews,
  selected = false,
  selectionMode = false,
  activePreviewId,
  previewResetEpoch,
  onPreviewStart,
  ratio,
  face,
}: {
  previews: LibraryGroup["previews"];
  selected?: boolean;
  selectionMode?: boolean;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  ratio: number | null;
  face: ReactNode;
}) {
  return (
    <VideoThumbnail selected={selected}>
      <FolderArt
        previews={previews}
        selectionMode={selectionMode}
        activePreviewId={activePreviewId}
        previewResetEpoch={previewResetEpoch}
        onPreviewStart={onPreviewStart}
      />
      <VideoThumbnailDuration className="pointer-events-none z-20">
        {face}
      </VideoThumbnailDuration>
      {ratio !== null && (
        <VideoThumbnailProgress
          value={Math.round(ratio * 100)}
          aria-label={t.library.group.watchedRatio}
          className="z-20"
        />
      )}
    </VideoThumbnail>
  );
}

/** CollectionName はグループとスマートフォルダのカードの名前である（2 行まで）。 */
export function CollectionName({ name, muted }: { name: string; muted: boolean }) {
  return (
    <h3
      title={name}
      className={cn(
        "line-clamp-2 text-sm font-medium break-all",
        muted ? "text-muted-foreground" : "text-foreground",
      )}
    >
      {name}
    </h3>
  );
}

/**
 * GroupCard はライブラリの格子でグループを1枚で出すカードである。動画のカード
 * （VideoCard）と同じ箱・同じ大きさで、サムネイルの枠にはフォルダ画面のフォルダカードと
 * 同じフォルダの絵柄（FolderArt）を描き、右下の面に本数と合計の長さを重ねる。
 * カード全体がグループのページへの1つのリンクである（specs/048-groups/research.md R-8。
 * 017 の「最初の未視聴のメンバーを開く」を置き換える）。
 */
export const GroupCard = memo(GroupCardFor) as typeof GroupCardFor;

/** groupHasTags は、グループがタグを持って届いた（ライブラリの LibraryGroup で、タグが 1 つ以上ある）かである。 */
function groupHasTags(group: LibraryGroupCard): boolean {
  return "tags" in group && Array.isArray(group.tags) && group.tags.length > 0;
}

function GroupCardFor<G extends LibraryGroupCard>(props: GroupCardProps<G>) {
  const {
    group,
    backTo,
    to = groupPagePath(group.id),
    selected,
    selectionMode,
    onSelect,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart,
    onPreviewReset,
    tagsRow,
    nameField,
    autoplay,
    favoriteOnly = false,
  } = props;
  const { state, ratio, duration, countText, label } = useGroupFacts(group);
  const owner = useAudience() === "owner";
  const tagsRowNode = tagsRow?.(group);
  const showTagsRow = tagsRow !== undefined && groupHasTags(group);

  return (
    <article
      data-group-id={group.id}
      data-group-root={group.folder?.rootId}
      data-group-path={group.folder?.path}
      className={cn(
        "group relative flex min-w-0 flex-col gap-2",
        selectionMode && "select-none",
      )}
    >
      {onSelect !== undefined && (
        // 前に出たサムネイルより上に置く（z-30）。
        <VideoThumbnailMark
          corner="top-start"
          className={cn(
            "z-30 p-1 transition-opacity duration-150",
            selectionMode || selected
              ? "opacity-100"
              : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100",
          )}
        >
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(group, next === true)}
            aria-label={t.library.group.select(group.name)}
            onClick={(event: MouseEvent) => event.stopPropagation()}
          />
        </VideoThumbnailMark>
      )}

      <Link
        to={to}
        state={cardLinkState(backTo, autoplay)}
        aria-label={label}
        onClick={(event) => {
          onPreviewReset?.();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(group, !selected);
          }
        }}
        className="flex min-w-0 flex-1 flex-col gap-2 rounded-md"
      >
        <CollectionThumbnail
          previews={group.previews}
          selected={selected}
          selectionMode={selectionMode}
          activePreviewId={activePreviewId}
          previewResetEpoch={previewResetEpoch}
          onPreviewStart={onPreviewStart}
          ratio={ratio}
          face={
            <>
              <span>{countText}</span>
              {duration !== "" && <span>{duration}</span>}
            </>
          }
        />

        {nameField === undefined && (
          <CollectionName name={group.name} muted={state === "watched"} />
        )}
      </Link>
      {nameField}
      {owner && (
        // 前に出たサムネイルより上（チェックと同じ z-30）。リンクの外、タグの行の前に置く
        // （ui-design.md「Placement」）。
        <VideoThumbnailMark className="z-30">
          {!favoriteOnly && <GroupAddToGroup group={group} variant="card" />}
          {!favoriteOnly && <GroupAddToPlaylist group={group} variant="card" />}
          <GroupFavorite group={group} variant="card" />
        </VideoThumbnailMark>
      )}
      {showTagsRow && <div className="flex min-w-0 flex-col gap-1">{tagsRowNode}</div>}
    </article>
  );
}

/**
 * GroupRow はリスト表示のグループの行である。動画の行（VideoRow）と同じ列に
 * グループの値を出す（ui-design.md「List view row」）。
 */
export const GroupRow = memo(GroupRowFor) as typeof GroupRowFor;

function GroupRowFor<G extends LibraryGroupCard>(props: GroupCardProps<G>) {
  const {
    group,
    backTo,
    to = groupPagePath(group.id),
    selected,
    selectionMode,
    onSelect,
    nameField,
    drag,
  } = props;
  const { watchedCount, state, ratio, duration, label } = useGroupFacts(group);
  const owner = useAudience() === "owner";
  // リスト表示は今回変えない。サムネイルは先頭のメンバーの1枚（previews の先頭）を使う。
  const cover = group.previews[0];

  return (
    <TableRow
      {...dragRowProps(drag, "group")}
      data-group-id={group.id}
      data-group-root={group.folder?.rootId}
      data-group-path={group.folder?.path}
      data-state={selected ? "selected" : undefined}
    >
      {onSelect !== undefined && (
        <TableCell className="w-10 pl-3">
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(group, next === true)}
            aria-label={t.library.group.select(group.name)}
            className={cn(
              "transition-opacity",
              selectionMode || selected
                ? "opacity-100"
                : "opacity-50 group-focus-within:opacity-100 group-hover:opacity-100",
            )}
          />
        </TableCell>
      )}
      <TableCell className="w-list-thumb-cell">
        <VideoThumbnail className="w-list-thumb rounded-sm">
          {cover !== undefined && <VideoThumbnailImage src={cover.thumbnailUrl} />}
          {ratio !== null && (
            <VideoThumbnailProgress
              value={Math.round(ratio * 100)}
              aria-label={t.library.group.watchedRatio}
            />
          )}
        </VideoThumbnail>
      </TableCell>
      <TableCell className="min-w-0 pr-4 whitespace-normal">
        {nameField ?? (
          <Link
            to={to}
            state={{ from: backTo }}
            aria-label={label}
            onClick={(event) => {
              if (selectionMode) {
                event.preventDefault();
                onSelect?.(group, !selected);
              }
            }}
            className={cn(
              "line-clamp-2 rounded-sm text-sm font-medium break-all hover:text-primary",
              state === "watched" ? "text-muted-foreground" : "text-foreground",
            )}
          >
            {group.name}
          </Link>
        )}
        <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <Folder aria-hidden="true" className="size-3 shrink-0" />
          {groupCountText(group.videoCount)}
        </span>
        <RowDropMarks drag={drag} />
      </TableCell>
      {owner && (
        <TableCell className="w-8 px-0">
          <GroupFavorite group={group} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <GroupAddToPlaylist group={group} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <GroupAddToGroup group={group} variant="row" />
        </TableCell>
      )}
      <CollectionValueCells
        state={state}
        watchedCount={watchedCount}
        videoCount={group.videoCount}
        duration={duration}
        sizeBytes={group.sizeBytes}
        addedAt={group.addedAt}
      />
    </TableRow>
  );
}

/**
 * CollectionValueCells はグループとスマートフォルダの行の、視聴・長さ・画質（空）・大きさ・
 * 追加日の列である（動画の行と同じ列）。
 */
export function CollectionValueCells({
  state,
  watchedCount,
  videoCount,
  duration,
  sizeBytes,
  addedAt,
}: {
  state: ReturnType<typeof useCollectionFacts>["state"];
  watchedCount: number;
  videoCount: number;
  duration: string;
  sizeBytes: number | undefined;
  addedAt: string | undefined;
}) {
  return (
    <>
      <TableCell className="hidden w-16 pr-4 text-right text-xs text-muted-foreground tabular-nums sm:table-cell">
        {state === "watched" && (
          <Check
            className="ml-auto size-4 text-success"
            aria-label={t.list.card.watched}
          />
        )}
        {state === "inProgress" && t.library.group.progress(watchedCount, videoCount)}
      </TableCell>
      <TableCell className="w-list-number pr-4 text-right tabular-nums">
        {duration}
      </TableCell>
      <TableCell className="hidden w-list-number pr-4 md:table-cell" />
      <TableCell className="hidden w-list-number-wide pr-4 text-right text-muted-foreground tabular-nums md:table-cell">
        {sizeBytes === undefined ? "" : formatBytes(sizeBytes)}
      </TableCell>
      <TableCell className="hidden w-list-date pr-3 text-right text-muted-foreground tabular-nums lg:table-cell">
        {addedAt === undefined ? "" : formatRelative(addedAt)}
      </TableCell>
    </>
  );
}
