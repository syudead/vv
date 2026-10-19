import {
  Activity,
  Building2,
  Folder,
  History,
  House,
  Layers,
  ListVideo,
  Tags,
  Users,
  type LucideIcon,
} from "lucide-react";

import { t, type UiText } from "../i18n";

export interface NavEntry {
  id: string;
  label: UiText;
  icon: LucideIcon;
  to: string;
  matchDescendants?: boolean;
  /** Hide owner-data navigation from guests. */
  ownerOnly?: boolean;
  attentionCount?: boolean;
}

/** Read catalog copy during rendering so pseudo-locale replacements propagate. */
export function navEntries(): readonly NavEntry[] {
  return [
    { id: "home", label: t.shell.nav.home, icon: House, to: "/" },
    {
      id: "folders",
      label: t.shell.nav.folders,
      icon: Folder,
      to: "/folders",
      matchDescendants: true,
    },
    {
      id: "history",
      label: t.shell.nav.history,
      icon: History,
      to: "/history",
      ownerOnly: true,
    },
    {
      id: "playlists",
      label: t.shell.nav.playlists,
      icon: ListVideo,
      to: "/playlists",
      ownerOnly: true,
    },
    { id: "tags", label: t.shell.nav.tags, icon: Tags, to: "/tags", ownerOnly: true },
    {
      id: "people",
      label: t.shell.nav.people,
      icon: Users,
      to: "/people",
      matchDescendants: true,
      ownerOnly: true,
    },
    {
      id: "organizations",
      label: t.shell.nav.organizations,
      icon: Building2,
      to: "/organizations",
      ownerOnly: true,
    },
    {
      id: "duplicates",
      label: t.shell.nav.duplicates,
      icon: Layers,
      to: "/duplicates",
      ownerOnly: true,
    },
    {
      id: "activity",
      label: t.shell.nav.activity,
      icon: Activity,
      to: "/activity",
      matchDescendants: true,
      ownerOnly: true,
      attentionCount: true,
    },
  ];
}
