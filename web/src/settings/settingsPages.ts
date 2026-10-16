import type { t } from "../i18n";

/**
 * 設定のページの対応表である（specs/058-settings-pages/research.md R-1、R-2）。ページは
 * `/settings/<slug>` のルートで、一覧はこの順に並ぶ。スラッグはカタログの
 * `settings.pages` のキー（一覧の項目、ページの題、ブラウザの題の 1 語）でもある。
 */
export type SettingsPageSlug = keyof typeof t.settings.pages;

/** SettingsSectionId はページに組み込む節である。部品との対応は SettingsPage が持つ。 */
export type SettingsSectionId =
  | "scan-status"
  | "auto-import-state"
  | "media-folders"
  | "auto-import-switch"
  | "version-judging"
  | "auto-tagging"
  | "ollama"
  | "comfyui"
  | "work-info-plugins"
  | "transcoding"
  | "network"
  | "api-tokens";

export interface SettingsPageEntry {
  slug: SettingsPageSlug;
  /** 上から順に組み込む節。 */
  sections: readonly SettingsSectionId[];
  /** `/settings#<anchor>` をこのページへ送る、節のアンカー（useSectionAnchor の id）。 */
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
    sections: ["media-folders", "auto-import-switch", "version-judging"],
    anchors: [],
  },
  { slug: "auto-tagging", sections: ["auto-tagging"], anchors: [] },
  { slug: "connections", sections: ["ollama", "comfyui"], anchors: ["comfyui"] },
  { slug: "plugins", sections: ["work-info-plugins"], anchors: ["work-info-plugins"] },
  { slug: "conversion", sections: ["transcoding"], anchors: [] },
  { slug: "network", sections: ["network"], anchors: [] },
  { slug: "api", sections: ["api-tokens"], anchors: [] },
];

/** defaultSettingsPage は `/settings` が sm から出し、知らないスラッグを置き換えるページである。 */
export const defaultSettingsPage: SettingsPageSlug = "scan";

/** settingsPagePath はページのアドレスである。`hash` は `#` 付きで渡す。 */
export function settingsPagePath(slug: SettingsPageSlug, hash = ""): string {
  return `/settings/${slug}${hash}`;
}

/**
 * SettingsRoute は設定のアドレスが何を出すかである。
 * - `list`: `/settings`。sm から既定のページを、sm 未満は一覧だけを出す。
 * - `page`: `/settings/<slug>`。
 * - `replace`: `to` へ置き換える（知らないスラッグ、古いアンカーのアドレス）。
 */
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

/** pageForAnchor は節のアンカー（`#` 付き）を持つページを返す。無ければ undefined。 */
export function pageForAnchor(hash: string): SettingsPageEntry | undefined {
  const anchor = hash.startsWith("#") ? hash.slice(1) : hash;
  if (anchor === "") return undefined;
  return settingsPages.find((page) => page.anchors.includes(anchor));
}

/**
 * settingsRoute は設定のアドレスのパス（`/settings` か、その下）とハッシュから、出すものを決める。
 * スラッグは React Router と同じく大文字と小文字を区別しない。パスから読むのは、設定のルートが
 * ほかのルートの `*` の下にあり、`useParams()["*"]` には外側の残りが入るからである。
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
