import { useEffect, useRef, useState } from "react";
import { Ellipsis, Folder, FolderMinus, Tag, Trash2 } from "lucide-react";
import { Link } from "react-router";

import { isAborted, RequestFailed, type Video } from "../api/client";
import { findVideoItem, removeFromGroup, taggedMessage } from "../api/groups";
import { clearListSnapshot } from "../api/listSnapshot";
import DissolveGroupDialog from "../groups/DissolveGroupDialog";
import GroupTagDialog from "../groups/GroupTagDialog";
import { groupPagePath } from "../groups/groupPaths";
import { errorText, t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";

type VideoGroup = NonNullable<Video["group"]>;

/**
 * GroupLine は題名の上に置く、グループ名と何本目かの1行である（specs/048-groups/ui-design.md
 * 「Video page」）。題名より小さく従の色にし、題名より先に目に入らないようにする。行はグループの
 * ページへのリンクで、ゲストにも同じリンクを出す。動画ページから開くので道筋は空である。
 *
 * 所有者には、行の後の同じ並びに「Group actions」のメニューを置く。行そのものはグループを開くので、
 * メニューは別のボタンにする（1 つの操作が開くこととメニューを開くことを兼ねない）。
 */
export default function GroupLine({
  group,
  videoId,
  owner,
  backTo,
  onChanged,
}: {
  group: VideoGroup;
  /** 今の動画の id。「Remove from this group」の項目を探すのに使う。 */
  videoId: number;
  owner: boolean;
  /** 動画ページの戻り先。グループのページへ `state.from` として引き継ぐ。 */
  backTo: string;
  /** グループの操作の後に、動画と関連動画を `group` なしで読み直させる。 */
  onChanged: () => void;
}) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Link
        to={groupPagePath(group.id)}
        state={{ from: backTo }}
        aria-label={t.player.group.line(group.name, group.position, group.count)}
        data-group-line=""
        className="flex max-w-full min-w-0 items-center gap-1.5 rounded-sm text-xs text-muted-foreground hover:text-foreground sm:text-sm"
      >
        <Folder className="size-4 shrink-0" aria-hidden="true" />
        <span className="truncate" title={group.name}>
          {group.name}
        </span>
        <span className="shrink-0" aria-hidden="true">
          ·
        </span>
        <span className="shrink-0 tabular-nums">
          {t.player.related.position(group.position, group.count)}
        </span>
      </Link>
      {owner && (
        <GroupActions
          // 別の動画・別のグループへ移ったら、探した項目と開いた窓を持ち越さない。
          key={`${String(group.id)}:${String(videoId)}`}
          group={group}
          videoId={videoId}
          onChanged={onChanged}
        />
      )}
    </div>
  );
}

/**
 * useDirectItem は、動画 `videoId` がグループ `groupId` の直接の項目ならその項目の id を返す。
 * 探している間、見つからないとき、読めなかったときは undefined（メニューに外す項目を出さない）。
 */
function useDirectItem(groupId: number, videoId: number): number | undefined {
  const [itemId, setItemId] = useState<number | undefined>(undefined);
  useEffect(() => {
    const controller = new AbortController();
    findVideoItem(groupId, videoId, controller.signal).then(
      setItemId,
      (error: unknown) => {
        if (!isAborted(error)) setItemId(undefined);
      },
    );
    return () => controller.abort();
  }, [groupId, videoId]);
  return itemId;
}

/**
 * GroupActions は所有者の「Group actions」のメニューである。「Remove from this group」（動画が
 * グループの直接の項目のときだけ）、「Turn the group into a tag…」、区切り、「Dissolve…」の順。
 * 解散とタグ化はグループのページと同じ窓を開く。どの操作も成功したらトーストを出し、ページに
 * 留まって動画と関連動画を読み直させる。
 */
function GroupActions({
  group,
  videoId,
  onChanged,
}: {
  group: VideoGroup;
  videoId: number;
  onChanged: () => void;
}) {
  const toast = useToast();
  const itemId = useDirectItem(group.id, videoId);
  const [dialog, setDialog] = useState<"tag" | "dissolve" | null>(null);
  const [removing, setRemoving] = useState(false);
  // 要求の間に別の動画へ移るか画面を離れたら、応答で読み直させない。読み直しは操作を始めた
  // ときの動画の URL へ移るので、利用者を前の動画へ引き戻してしまう。
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const changed = () => {
    if (mounted.current) onChanged();
  };

  const remove = (id: number) => {
    setRemoving(true);
    removeFromGroup(group.id, [id]).then(
      () => {
        setRemoving(false);
        clearListSnapshot();
        toast(t.player.group.removed(group.name));
        changed();
      },
      (error: unknown) => {
        setRemoving(false);
        // 項目かグループがもう無い。トーストなしで読み直し、今の所属に合わせる。
        if (error instanceof RequestFailed && error.status === 404) {
          clearListSnapshot();
          changed();
          return;
        }
        toast(t.player.group.removeFailed(errorText(error)));
      },
    );
  };

  // 窓が途中で解散を知ったときは、トーストなしで読み直す。
  const gone = () => {
    setDialog(null);
    changed();
  };

  return (
    <>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={removing}
                aria-label={t.player.group.actions}
                className="shrink-0 text-muted-foreground"
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>{t.player.group.actions}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="start">
          {itemId !== undefined && (
            <DropdownMenuItem onSelect={() => remove(itemId)}>
              <FolderMinus aria-hidden="true" />
              {t.player.group.remove}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => setDialog("tag")}>
            <Tag aria-hidden="true" />
            {t.player.group.toTag}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setDialog("dissolve")}>
            <Trash2 aria-hidden="true" />
            {t.player.group.dissolve}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog === "tag" && (
        <GroupTagDialog
          group={group}
          onClose={() => setDialog(null)}
          onTagged={(result) => {
            setDialog(null);
            toast(taggedMessage(result));
            changed();
          }}
          onGone={gone}
        />
      )}
      {dialog === "dissolve" && (
        <DissolveGroupDialog
          group={group}
          onClose={() => setDialog(null)}
          onDissolved={() => {
            setDialog(null);
            toast(t.groups.dissolved(group.name));
            changed();
          }}
          onGone={gone}
        />
      )}
    </>
  );
}
