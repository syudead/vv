import { FolderSearch } from "lucide-react";
import { memo, type ReactNode } from "react";
import { Link } from "react-router";

import type { SmartFolderSummary, VideoTag } from "../api/client";
import { smartFolderKey } from "../api/libraryItems";
import type { LibrarySmartFolder } from "../api/smartFolders";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import {
  CollectionName,
  CollectionThumbnail,
  CollectionValueCells,
  useCollectionFacts,
} from "../library/GroupCard";
import { Skeleton } from "../ui/shadcn/skeleton";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
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
 * SmartFolderCard はスマートフォルダを 1 枚で出すカードである。箱・絵柄・本数の面・視聴の帯・
 * 名前・タグの行はグループのカードと同じで、違いは面の本数の前の印だけ。チェックとドラッグは
 * 持たない（中の動画はそれぞれのカードで選ぶ）。印（お気に入り、プレイリスト）は再生の単位が
 * 足す。値が届く前は本数の代わりに骨組みを出し、答えに入らなかったフォルダは描かない。
 */
export const SmartFolderCard = memo(function SmartFolderCard(
  props: SmartFolderCardProps,
) {
  const { folder, backTo, tagsRow, refusesDrop = false } = props;
  const { gone, values, facts, label, ref } = useSmartFolderFacts(folder);
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
 * 印が `FolderSearch` になり、チェック・お気に入り・プレイリスト・グループの列は空にする
 * （`owner` のときだけその列がある）。
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
      {owner && <TableCell className="w-8 px-0" />}
      {owner && <TableCell className="hidden w-8 px-0 sm:table-cell" />}
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
