import { Folder, Library, Tags, type LucideIcon } from "lucide-react";

import { t, type UiText } from "../i18n";

export interface NavEntry {
  id: string;
  label: UiText;
  icon: LucideIcon;
  /** 行き先。 */
  to: string;
  /** 行き先より下の URL（`/folders/3/A` など）でも選択中にする。 */
  matchDescendants?: boolean;
  /**
   * 所有者のデータ（タグ）に依る項目。ゲストには出さない
   * （specs/016-single-account-auth/ui-design.md「Guest degradation」）。
   */
  ownerOnly?: boolean;
}

/**
 * navEntries はサイドバーの上段の項目である。文言はカタログから描画のたびに引く
 * （疑似ロケールへの差し替えが届くよう、モジュールの読み込み時に固めない）。
 */
export function navEntries(): readonly NavEntry[] {
  return [
    { id: "library", label: t.shell.nav.library, icon: Library, to: "/" },
    {
      id: "folders",
      label: t.shell.nav.folders,
      icon: Folder,
      to: "/folders",
      matchDescendants: true,
    },
    { id: "tags", label: t.shell.nav.tags, icon: Tags, to: "/tags", ownerOnly: true },
  ];
}
