import { Ellipsis, Pencil, Tag, Trash2 } from "lucide-react";
import { type Ref, useState } from "react";

import type { LibraryGroup } from "../api/client";
import { renameGroup, taggedMessage } from "../api/groups";
import { t, type UiText } from "../i18n";
import { playlistName } from "../playlists/playlistName";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import RenameDialog from "../videoList/RenameDialog";
import DissolveGroupDialog from "./DissolveGroupDialog";
import GroupTagDialog from "./GroupTagDialog";

type Dialog = "rename" | "tag" | "dissolve" | null;

/**
 * GroupMenu は所有者のグループのページの見出しの「More」である（specs/048-groups/ui-design.md
 * 「Header」）。「Rename…」「Turn the group into a tag…」、区切り、「Dissolve…」を開き、それぞれの
 * 窓を持つ。
 *
 * 改名できたら `onRenamed` に新しい値を渡し、フォーカスは「More」へ戻す。グループが終わったら
 * （解散、タグ化）`onLeave` で道筋の親かライブラリへ移り、そこでトーストを出す。もう無かった
 * （`404`）とき、解散は同じく移ってトーストを出さず、改名とタグ化は `onGone` で無くなった状態を
 * 見せる。
 */
export default function GroupMenu({
  group,
  moreRef,
  onRenamed,
  onLeave,
  onGone,
}: {
  group: LibraryGroup;
  moreRef: Ref<HTMLButtonElement>;
  onRenamed: (group: LibraryGroup) => void;
  /** 解散かタグ化の後に画面を離れる（useLeaveGroup）。 */
  onLeave: (message?: UiText) => void;
  onGone: () => void;
}) {
  const [dialog, setDialog] = useState<Dialog>(null);

  const leave = (message?: UiText) => {
    setDialog(null);
    onLeave(message);
  };
  const gone = () => {
    setDialog(null);
    onGone();
  };

  return (
    <>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                ref={moreRef}
                variant="ghost"
                size="icon-sm"
                aria-label={t.common.more}
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>{t.common.more}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => setDialog("rename")}>
            <Pencil aria-hidden="true" />
            {t.groups.detail.rename}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setDialog("tag")}>
            <Tag aria-hidden="true" />
            {t.groups.detail.toTag}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setDialog("dissolve")}>
            <Trash2 aria-hidden="true" />
            {t.groups.detail.dissolve}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog === "rename" && (
        <RenameDialog
          currentName={group.name}
          texts={{
            ...t.groups.renameDialog,
            nameLabel: t.groups.nameLabel,
            nameRule: t.groups.nameRule,
          }}
          // グループの名前の規則はプレイリストと同じ（research.md R-14）。
          normalize={playlistName}
          rename={(name) => renameGroup(group.id, name)}
          onClose={() => setDialog(null)}
          onRenamed={(renamed) => {
            setDialog(null);
            onRenamed(renamed);
          }}
          onGone={gone}
        />
      )}
      {dialog === "tag" && (
        <GroupTagDialog
          group={group}
          onClose={() => setDialog(null)}
          onTagged={(result) => leave(taggedMessage(result))}
          onGone={gone}
        />
      )}
      {dialog === "dissolve" && (
        <DissolveGroupDialog
          group={group}
          onClose={() => setDialog(null)}
          onDissolved={() => leave(t.groups.dissolved(group.name))}
          onGone={() => leave()}
        />
      )}
    </>
  );
}
