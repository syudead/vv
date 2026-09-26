import { ChevronDown, Group, LoaderCircle, Tag, Ungroup } from "lucide-react";
import { useState } from "react";

import type { FolderRef } from "../api/client";
import {
  type FolderGrouping,
  type FolderGroupingMode,
  groupingFailure,
  setFolderGrouping,
  taggedMessage,
  tagFolderGroup,
} from "../api/folderGrouping";
import Button from "../ui/Button";
import {
  MenuContent,
  MenuItem,
  MenuLabel,
  MenuRadioGroup,
  MenuRadioItem,
  MenuRoot,
  MenuSeparator,
  MenuTrigger,
} from "../ui/Menu";
import { useToast } from "../ui/Toast";

const modes: { value: FolderGroupingMode; label: string }[] = [
  { value: "auto", label: "自動" },
  { value: "ungroup", label: "まとめを解除" },
  { value: "groupDirect", label: "直下をまとめる" },
];

/**
 * FolderGroupingMenu はフォルダ画面の「動画 N」の見出しの行の右端に置く、そのフォルダの
 * ライブラリでのまとめ方のメニューである（specs/017-folder-groups/ui-design.md
 * 「Folder grouping menu」、要件 7・11・22）。
 *
 * ラジオは応答の `grouping` を `onGroupingChange` で返すだけで、一覧は読み直させない。
 * タグ化の成功は `onTagged` で、画面に動画の一覧を読み直させる。409 は `onConflict` で
 * フォルダの一覧を取り直させる。どちらの操作も、成功すると一覧の控えを捨てる（api/folderGrouping）。
 */
export default function FolderGroupingMenu({
  folder,
  name,
  grouping,
  onGroupingChange,
  onTagged,
  onConflict,
}: {
  folder: FolderRef;
  /** フォルダの名前（失敗の文言に使う）。 */
  name: string;
  grouping: FolderGrouping;
  onGroupingChange: (grouping: FolderGrouping) => void;
  onTagged: () => void;
  onConflict: () => void;
}) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  const fail = (failure: unknown, tagging: boolean) => {
    const { message, conflict } = groupingFailure(failure, {
      name,
      tagging,
      notFoundMessage: "このフォルダは見つかりません",
    });
    toast(message);
    if (conflict) onConflict();
  };

  // 同じ値を選び直しても送る（ui-design.md「Folder grouping menu」。応答は 200）。
  const choose = async (mode: FolderGroupingMode) => {
    setBusy(true);
    try {
      onGroupingChange(await setFolderGrouping(folder, mode));
    } catch (failure) {
      fail(failure, false);
    } finally {
      setBusy(false);
    }
  };

  const toTag = async () => {
    setBusy(true);
    try {
      const result = await tagFolderGroup(folder);
      toast(taggedMessage(result));
      onGroupingChange(result.grouping);
      onTagged();
    } catch (failure) {
      fail(failure, true);
    } finally {
      setBusy(false);
    }
  };

  const Icon = busy ? LoaderCircle : grouping.grouped ? Group : Ungroup;
  return (
    <MenuRoot>
      <MenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          aria-label={`ライブラリでのまとめ方: ${
            grouping.grouped ? "1 件にまとめて表示" : "1 本ずつ表示"
          }。メニューを開く`}
          // 見出しと同じ弱さにし、見出しの行の高さを変えない（ui-design.md「Visual review criteria」）。
          className="-my-2 -mr-2 gap-1.5! text-fg-muted!"
        >
          <Icon aria-hidden="true" className={busy ? "animate-spin" : undefined} />
          {grouping.grouped ? "ライブラリで 1 件" : "ライブラリで 1 本ずつ"}
          <ChevronDown aria-hidden="true" className="size-3.5!" />
        </Button>
      </MenuTrigger>
      <MenuContent align="end" className="max-w-80">
        <MenuLabel>ライブラリでのまとめ方</MenuLabel>
        <MenuRadioGroup value={grouping.mode} onValueChange={(mode) => void choose(mode)}>
          {modes.map((mode) => (
            <MenuRadioItem key={mode.value} value={mode.value}>
              {mode.label}
            </MenuRadioItem>
          ))}
        </MenuRadioGroup>
        <p className="px-2.5 pt-1 pb-1.5 text-xs text-fg-muted">
          自動: 登録フォルダより下で、子フォルダが無く動画が 2 本以上のフォルダをまとめる
        </p>
        {grouping.taggable && (
          <>
            <MenuSeparator />
            <MenuItem onSelect={() => void toTag()}>
              <Tag aria-hidden="true" />
              グループをタグに変える
            </MenuItem>
          </>
        )}
      </MenuContent>
    </MenuRoot>
  );
}
