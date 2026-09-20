import { Clock, History, Library, type LucideIcon } from "lucide-react";

export interface NavEntry {
  id: string;
  label: string;
  icon: LucideIcon;
  /** 行き先。無ければ「準備中」として扱い、押すとトーストを出す。 */
  to?: string;
}

export const navEntries: readonly NavEntry[] = [
  { id: "library", label: "ライブラリ", icon: Library, to: "/" },
  { id: "recent", label: "最近追加", icon: Clock },
  { id: "in-progress", label: "視聴途中", icon: History },
];
