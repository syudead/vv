import { FolderSearch } from "lucide-react";
import { memo, type ReactNode, useState } from "react";
import { Link } from "react-router";

import type { SmartFolderSummary, VideoTag } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { smartFolderKey } from "../api/libraryItems";
import type { LibrarySmartFolder } from "../api/smartFolders";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import {
  CollectionName,
  CollectionThumbnail,
  CollectionValueCells,
  useCollectionFacts,
} from "../library/GroupCard";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { Skeleton } from "../ui/shadcn/skeleton";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import {
  VideoThumbnail,
  VideoThumbnailImage,
  VideoThumbnailMark,
} from "../ui/VideoThumbnail";
import { RefusingDropCard } from "../videoList/cardDrag";
import { useSmartFolderCardValue } from "./cardValues";
import { smartFolderPagePath } from "./smartFolderPaths";

/**
 * SmartFolderCardProps はスマートフォルダのカードと行の props である
 * （specs/052-smart-folders/ui-design.md「Smart folder card and row」）。項目は要約だけを持ち、
 * 値はカードが画面に入ってから SmartFolderCardsProvider が読む。
 */
export interface SmartFolderCardProps {
  folder: SmartFolderSummary;
  /** 遷移元の一覧 URL。開いた先の「戻る」がここへ帰る。 */
  backTo: string;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  onPreviewReset?: () => void;
  /** 題名の下のタグの行（格子表示だけ）。 */
  tagsRow?: (tags: readonly VideoTag[]) => ReactNode;
  /**
   * 所有者のドラッグの層の中で描くとき true。カードは持ち上げられず、上に落とすと拒否する
   * （RefusingDropCard）。
   */
  refusesDrop?: boolean;
}

/** noValues は値が届く前の視聴の値である（本数 0、視聴の帯なし）。 */
const noValues = { videoCount: 0 };

/**
 * useSmartFolderFacts はカードの値と、それから決める視聴の値・読み上げ名である。値が届く前は
 * `values` が undefined で、読み上げ名は名前と「smart folder」だけ。
 */
function useSmartFolderFacts(folder: SmartFolderSummary) {
  const { value, ref } = useSmartFolderCardValue(folder.id);
  const values: LibrarySmartFolder | undefined = value === "gone" ? undefined : value;
  const facts = useCollectionFacts(values ?? noValues);
  const label =
    values === undefined
      ? t.library.smartFolder.labelLoading(folder.name)
      : t.library.smartFolder.label(folder.name, values.videoCount, facts.watchedCount);
  return { gone: value === "gone", values, facts, label, ref };
}

/**
 * SmartFolderMark はカードの面と行の本数の前に置くスマートフォルダの印である。グループの
 * カードとの見た目の違いはこの小さな印だけで、休むと「Smart folder」を出す。
 */
function SmartFolderMark() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span title={t.smartFolders.kind} className="pointer-events-auto flex">
          <FolderSearch aria-hidden="true" className="size-3 shrink-0" />
        </span>
      </TooltipTrigger>
      <TooltipContent>{t.smartFolders.kind}</TooltipContent>
    </Tooltip>
  );
}

/**
 * SmartFolderFavorite はフォルダのカード・行のお気に入りの付け外しである（所有者だけ。
 * specs/052-smart-folders/ui-design.md「Smart folder card and row」）。フォルダの id を
 * `smartFolderIds` で送る。フォルダの付け外しは画面へ知らせない（api/favorites.ts）ので、
 * 成功したらここで塗りを変える。この塗りは押したときの `read` にだけ効き、カードの値か要約が
 * 新しく読まれたら（`read` が別の物になったら）捨てて、読んだ `favorite` に従う。値が同じかは
 * 比べない（別タブが元へ戻した値を、古い塗りが隠さないため）。
 */
function SmartFolderFavorite({
  folder,
  read,
  variant,
}: {
  folder: SmartFolderSummary;
  /** いちばん新しく読んだ値。カードの値が届く前は要約。読み直すたびに別の物になる。 */
  read: { favorite?: boolean };
  variant: "card" | "row";
}) {
  const [toggled, setToggled] = useState<{ read: object; favorite: boolean }>();
  const favorite =
    toggled !== undefined && toggled.read === read
      ? toggled.favorite
      : read.favorite === true;
  return (
    <FavoriteToggle
      favorite={favorite}
      label={t.library.smartFolder.favorite(folder.name)}
      onToggle={async () => {
        await updateFavorites([], [], !favorite, { smartFolderIds: [folder.id] });
        setToggled({ read, favorite: !favorite });
      }}
      variant={variant}
    />
  );
}

/**
 * SmartFolderAddToPlaylist はフォルダのカード・行の「Add to playlist」である（所有者だけ。
 * specs/052-smart-folders/ui-design.md「Add to playlist」）。フォルダを 1 つの項目として
 * `smartFolderIds` で送る。
 */
function SmartFolderAddToPlaylist({
  folder,
  variant,
}: {
  folder: SmartFolderSummary;
  variant: "card" | "row";
}) {
  return (
    <AddToPlaylistButton
      variant={variant}
      label={t.library.smartFolder.addToPlaylist(folder.name)}
      smartFolderIds={[folder.id]}
    />
  );
}

/**
 * SmartFolderCard はスマートフォルダを 1 枚で出すカードである。箱・絵柄・本数の面・視聴の帯・
 * 名前・タグの行はグループのカードと同じで、違いは面の本数の前の印だけ。チェックとドラッグは
 * 持たない（中の動画はそれぞれのカードで選ぶ）。所有者には角の印にハートと「Add to playlist」を
 * 置き、「Add to group」は置かない（フォルダはグループに入らない）。値が届く前は本数の代わりに
 * 骨組みを出し、答えに入らなかったフォルダは描かない。
 */
export const SmartFolderCard = memo(function SmartFolderCard(
  props: SmartFolderCardProps,
) {
  const { folder, backTo, tagsRow, refusesDrop = false } = props;
  const { gone, values, facts, label, ref } = useSmartFolderFacts(folder);
  const owner = useAudience() === "owner";
  if (gone) return null;
  const empty = values?.videoCount === 0;
  const tags = values?.tags ?? [];

  const card = (
    <article
      ref={ref}
      data-smart-folder-id={folder.id}
      className="group relative flex min-w-0 flex-col gap-2"
    >
      <Link
        to={smartFolderPagePath(folder.id)}
        state={{ from: backTo }}
        aria-label={label}
        onClick={() => props.onPreviewReset?.()}
        className="flex min-w-0 flex-1 flex-col gap-2 rounded-md"
      >
        <CollectionThumbnail
          previews={values?.previews ?? []}
          activePreviewId={props.activePreviewId}
          previewResetEpoch={props.previewResetEpoch}
          onPreviewStart={props.onPreviewStart}
          ratio={facts.ratio}
          face={
            <>
              <SmartFolderMark />
              {values === undefined ? (
                <Skeleton aria-hidden="true" className="h-3 w-16" />
              ) : (
                <>
                  <span>{facts.countText}</span>
                  {facts.duration !== "" && <span>{facts.duration}</span>}
                </>
              )}
            </>
          }
        />
        <CollectionName name={folder.name} muted={facts.state === "watched" || empty} />
      </Link>
      {owner && (
        // グループのカードと同じく、前に出たサムネイルより上に、リンクの外のタグの行の前に置く。
        <VideoThumbnailMark className="z-30">
          <SmartFolderAddToPlaylist folder={folder} variant="card" />
          <SmartFolderFavorite folder={folder} read={values ?? folder} variant="card" />
        </VideoThumbnailMark>
      )}
      {tagsRow !== undefined && tags.length > 0 && (
        <div className="flex min-w-0 flex-col gap-1">{tagsRow(tags)}</div>
      )}
    </article>
  );
  return refusesDrop ? (
    <RefusingDropCard id={smartFolderKey(folder.id)}>{card}</RefusingDropCard>
  ) : (
    card
  );
});

/**
 * SmartFolderRow はリスト表示のスマートフォルダの行である。グループの行と同じ列で、本数の行の
 * 印が `FolderSearch` になる。チェックとグループの列は空にし、お気に入りとプレイリストの列に
 * それぞれの操作を置く（`owner` のときだけその列がある）。
 */
export const SmartFolderRow = memo(function SmartFolderRow({
  folder,
  backTo,
  owner,
}: SmartFolderCardProps & { owner: boolean }) {
  const { gone, values, facts, label, ref } = useSmartFolderFacts(folder);
  if (gone) return null;
  const cover = values?.previews[0];
  const empty = values?.videoCount === 0;
  return (
    <TableRow ref={ref} data-smart-folder-id={folder.id}>
      {owner && <TableCell className="w-10 pl-3" />}
      <TableCell className="w-list-thumb-cell">
        <VideoThumbnail className="w-list-thumb rounded-sm">
          {cover !== undefined && <VideoThumbnailImage src={cover.thumbnailUrl} />}
        </VideoThumbnail>
      </TableCell>
      <TableCell className="min-w-0 pr-4 whitespace-normal">
        <Link
          to={smartFolderPagePath(folder.id)}
          state={{ from: backTo }}
          aria-label={label}
          className={cn(
            "line-clamp-2 rounded-sm text-sm font-medium break-all hover:text-primary",
            facts.state === "watched" || empty
              ? "text-muted-foreground"
              : "text-foreground",
          )}
        >
          {folder.name}
        </Link>
        <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <SmartFolderMark />
          {values === undefined ? (
            <Skeleton aria-hidden="true" className="h-3 w-16" />
          ) : (
            facts.countText
          )}
        </span>
      </TableCell>
      {owner && (
        <TableCell className="w-8 px-0">
          <SmartFolderFavorite folder={folder} read={values ?? folder} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <SmartFolderAddToPlaylist folder={folder} variant="row" />
        </TableCell>
      )}
      {owner && <TableCell className="hidden w-8 px-0 sm:table-cell" />}
      <CollectionValueCells
        state={facts.state}
        watchedCount={facts.watchedCount}
        videoCount={values?.videoCount ?? 0}
        duration={values === undefined ? "" : facts.duration}
        sizeBytes={values?.sizeBytes}
        addedAt={values?.addedAt}
      />
    </TableRow>
  );
});
