import { Clock, Folder, History, Library, type LucideIcon } from "lucide-react";

export interface NavEntry {
  id: string;
  label: string;
  icon: LucideIcon;
  /** 行き先。無ければ「準備中」として扱い、押すとトーストを出す。 */
  to?: string;
  /** 行き先より下の URL（`/folders/3/A` など）でも選択中にする。 */
  matchDescendants?: boolean;
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
  { id: "recent", label: "最近追加", icon: Clock },
  { id: "in-progress", label: "視聴途中", icon: History },
];
