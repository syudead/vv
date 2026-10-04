import { useState, type ReactNode } from "react";
import { ChevronDown, Folder, Tag, Ungroup } from "lucide-react";

import type { Video } from "../api/client";
import {
  groupingFailure,
  setFolderGrouping,
  taggedMessage,
  tagFolderGroup,
  ungroupedMessage,
} from "../api/folderGrouping";
import { t } from "../i18n";
import Button from "../ui/Button";
import { MenuContent, MenuItem, MenuRoot, MenuTrigger } from "../ui/legacy/Menu";
import { useToast } from "../ui/legacy/Toast";

/**
 * GroupLine は題名の上に置く、グループ名と何本目かの1行である（要件 25、ui-design.md
 * 「Group line」）。題名より小さく従の色にし、題名より先に目に入らないようにする。
 * 見出しの帯のパンくずが同じフォルダへのリンクを持つので、グループ名はリンクにしない。
 *
 * 所有者では行全体をまとめ方のメニューの引き金にする（要件 29）。ゲストでは押せない文字の行。
 */
export default function GroupLine({
  group,
  owner,
  onChanged,
}: {
  group: NonNullable<Video["group"]>;
  owner: boolean;
  /** まとめ方を変えた後（と 409 の後）に、動画と関連動画を取り直させる。 */
  onChanged: () => void;
}) {
  const content = (
    <>
      <Folder className="size-3.5! shrink-0 text-fg-subtle" aria-hidden="true" />
      <span className="truncate" title={group.name}>
        {group.name}
      </span>
      <span className="shrink-0 text-fg-subtle" aria-hidden="true">
        ·
      </span>
      <span className="shrink-0 tabular-nums">
        {t.player.related.position(group.position, group.count)}
      </span>
    </>
  );
  if (!owner) {
    return (
      <p className="flex min-w-0 items-center gap-1.5 text-xs text-fg-muted sm:text-sm">
        {content}
      </p>
    );
  }
  return <GroupLineMenu group={group} onChanged={onChanged} content={content} />;
}

/** GroupLineMenu は所有者の Group line で、「まとめを解除」「グループをタグに変える」を開く。 */
function GroupLineMenu({
  group,
  onChanged,
  content,
}: {
  group: NonNullable<Video["group"]>;
  onChanged: () => void;
  content: ReactNode;
}) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const folder = { rootId: group.folder.rootId, path: group.folder.path };

  const run = async (tagging: boolean) => {
    setBusy(true);
    try {
      if (tagging) {
        toast(taggedMessage(await tagFolderGroup(folder)));
      } else {
        await setFolderGrouping(folder, "ungroup");
        toast(ungroupedMessage(group.name));
      }
      onChanged();
    } catch (failure) {
      const { message, conflict } = groupingFailure(failure, {
        name: group.name,
        tagging,
        notFoundMessage: t.folderGrouping.changeFailed,
      });
      toast(message);
      if (conflict) onChanged();
    } finally {
      setBusy(false);
    }
  };

  return (
    <MenuRoot>
      <MenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          aria-label={t.player.group.menu(group.name, group.position, group.count)}
          // 文字の左端を題名にそろえ、行は題名より小さく従の色のままにする。
          className="-ml-2 max-w-full min-w-0 self-start gap-1.5! font-normal! text-fg-muted! sm:text-sm!"
        >
          {content}
          <ChevronDown className="size-3.5! shrink-0" aria-hidden="true" />
        </Button>
      </MenuTrigger>
      <MenuContent align="start">
        <MenuItem onSelect={() => void run(false)}>
          <Ungroup aria-hidden="true" />
          {t.player.group.ungroup}
        </MenuItem>
        {/* 登録フォルダそのもののグループはタグに変えられない（contracts §2）。 */}
        {group.folder.path !== "" && (
          <MenuItem onSelect={() => void run(true)}>
            <Tag aria-hidden="true" />
            {t.player.group.toTag}
          </MenuItem>
        )}
      </MenuContent>
    </MenuRoot>
  );
}
