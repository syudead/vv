import { ChevronDown, Globe, Lock } from "lucide-react";

import { maxVideoTagsSelection } from "../api/tags";
import { updateVideoVisibility } from "../api/visibility";
import { errorText, t } from "../i18n";
import Button from "../ui/Button";
import { MenuContent, MenuItem, MenuRoot, MenuTrigger } from "../ui/Menu";
import { useToast } from "../ui/Toast";
import { overLimitMessage } from "./selectionErrors";

/**
 * VisibilityMenu は選択バーの「公開」である（specs/016-single-account-auth/ui-design.md
 * 「Visibility toggle」の「Selection bar」）。選んだ動画の今の状態は示さず、
 * 「公開にする」「非公開にする」の両方を常に押せる。確定したらトーストで結果を伝え、
 * 選択は残す。カードの印は応答の通知（api/visibility.ts）で一覧が差し替える。
 */
export default function VisibilityMenu({
  selectedIds,
  overLimit,
  overLimitId,
  className,
}: {
  selectedIds: readonly number[];
  overLimit: boolean;
  overLimitId: string;
  className?: string;
}) {
  const toast = useToast();

  function apply(isPublic: boolean) {
    // 送る直前に選択の最新の件数を確かめる（タグの一括操作と同じ理由。上限を超えると
    // 全部か無しかで 400 になる）。
    if (selectedIds.length > maxVideoTagsSelection) {
      toast(overLimitMessage());
      return;
    }
    void updateVideoVisibility(selectedIds, isPublic).then(
      (result) => {
        toast(
          isPublic
            ? t.library.selection.madePublic(result.applied)
            : t.library.selection.madePrivate(result.applied),
        );
      },
      (error: unknown) => toast(t.library.selection.visibilityFailed(errorText(error))),
    );
  }

  return (
    <MenuRoot>
      <MenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={className}
          disabled={overLimit}
          title={overLimit ? overLimitMessage() : undefined}
          aria-describedby={overLimit ? overLimitId : undefined}
        >
          <Globe aria-hidden="true" />
          {t.library.selection.visibility}
          <ChevronDown aria-hidden="true" />
        </Button>
      </MenuTrigger>
      <MenuContent side="top" align="start">
        <MenuItem onSelect={() => apply(true)}>
          <Globe aria-hidden="true" />
          {t.library.selection.makePublic}
        </MenuItem>
        <MenuItem onSelect={() => apply(false)}>
          <Lock aria-hidden="true" />
          {t.library.selection.makePrivate}
        </MenuItem>
      </MenuContent>
    </MenuRoot>
  );
}
