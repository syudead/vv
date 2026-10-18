import type { t } from "../i18n";

/**
 * Navigation order and route slugs share settings.pages catalog keys for navigation, heading and
 * browser title.
 */
export type SettingsPageSlug = keyof typeof t.settings.pages;

export type SettingsSectionId =
  | "scan-status"
  | "auto-import-state"
  | "media-folders"
  | "auto-import-switch"
  | "version-judging"
  | "setup"
  | "auto-tagging"
  | "ollama"
  | "comfyui"
  | "work-info-plugins"
  | "transcoding"
  | "network"
  | "api-tokens";

export interface SettingsPageEntry {
  slug: SettingsPageSlug;
  sections: readonly SettingsSectionId[];
  /** Legacy /settings#anchor redirects to this page. */
  anchors: readonly string[];
}

export const settingsPages: readonly SettingsPageEntry[] = [
  {
    slug: "scan",
    sections: ["scan-status", "auto-import-state"],
    anchors: ["scan-status"],
  },
  {
    slug: "library",

    sections: ["media-folders", "auto-import-switch", "version-judging", "setup"],
    anchors: [],
  },
  { slug: "auto-tagging", sections: ["auto-tagging"], anchors: [] },
  { slug: "connections", sections: ["ollama", "comfyui"], anchors: ["comfyui"] },
  { slug: "plugins", sections: ["work-info-plugins"], anchors: ["work-info-plugins"] },
  { slug: "conversion", sections: ["transcoding"], anchors: [] },
  { slug: "network", sections: ["network"], anchors: [] },
  { slug: "api", sections: ["api-tokens"], anchors: [] },
];

export const defaultSettingsPage: SettingsPageSlug = "scan";

/** hash includes the leading #. */
export function settingsPagePath(slug: SettingsPageSlug, hash = ""): string {
  return `/settings/${slug}${hash}`;
}

export type SettingsRoute =
  | { kind: "list"; page: SettingsPageEntry }
  | { kind: "page"; page: SettingsPageEntry }
  | { kind: "replace"; to: string };

function entryFor(slug: string): SettingsPageEntry | undefined {
  return settingsPages.find((page) => page.slug === slug);
}

function defaultEntry(): SettingsPageEntry {
  const entry = entryFor(defaultSettingsPage);
  if (entry === undefined) throw new Error("the default settings page is missing");
  return entry;
}

export function pageForAnchor(hash: string): SettingsPageEntry | undefined {
  const anchor = hash.startsWith("#") ? hash.slice(1) : hash;
  if (anchor === "") return undefined;
  return settingsPages.find((page) => page.anchors.includes(anchor));
}

/**
 * Read the pathname rather than nested splat params, which contain the outer remainder; match
 * React Router's case-insensitivity.
 */
export function settingsRoute(pathname: string, hash: string): SettingsRoute {
  const slug = pathname
    .toLowerCase()
    .replace(/^\/settings(\/|$)/, "")
    .replace(/\/+$/, "");
  if (slug === "") {
    const anchored = pageForAnchor(hash);
    if (anchored !== undefined)
      return { kind: "replace", to: settingsPagePath(anchored.slug, hash) };
    return { kind: "list", page: defaultEntry() };
  }
  const entry = entryFor(slug);
  if (entry === undefined)
    return { kind: "replace", to: settingsPagePath(defaultSettingsPage) };
  return { kind: "page", page: entry };
}
