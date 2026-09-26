import { Folder, Library, Tags, type LucideIcon } from "lucide-react";

export interface NavEntry {
  id: string;
  label: string;
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

export const navEntries: readonly NavEntry[] = [
  { id: "library", label: "ライブラリ", icon: Library, to: "/" },
  {
    id: "folders",
    label: "フォルダ",
    icon: Folder,
    to: "/folders",
    matchDescendants: true,
  },
  { id: "tags", label: "タグ", icon: Tags, to: "/tags", ownerOnly: true },
];
