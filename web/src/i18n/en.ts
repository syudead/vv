import type { components } from "../api/gen/openapi";
import { formatNumber, selectPlural } from "./intl";

// 英語の文言カタログである。画面の文言はここに集め、コンポーネントは messages.ts の `t`
// から引く（specs/023-english-i18n/research.md R-1）。
//
// - 領域ごとの入れ子のオブジェクトにする。後続の単位が自分の領域の枝を足す。
// - 埋め込みを持つ文言は引数を取る関数にする。数は formatNumber、単数・複数は
//   selectPlural を通す（R-2）。
// - 利用者のデータ（名前、パス）は翻訳せず、そのまま埋め込む。
// - 既知のエラーの表は生成型の Record にするので、API に足したコードや理由の文が無いと
//   型検査で落ちる（R-3）。

type ErrorCode = components["schemas"]["Error"]["code"];
type ErrorReason = components["schemas"]["ErrorReason"];
type ProbeErrorCode = components["schemas"]["ProbeErrorCode"];
type ScanErrorCode = components["schemas"]["ScanErrorCode"];

/** ErrorDetails は API エラーが文言へ渡す値である（contracts/error-api.md §1）。 */
export interface ErrorDetails {
  limit?: number;
  tagName?: string;
}

/** ErrorEntry は既知のエラー 1 つの文である。値を埋め込む文は関数にする。 */
export type ErrorEntry = string | ((details: ErrorDetails) => string);

function characters(limit: number): string {
  return selectPlural(limit, {
    one: `${formatNumber(limit)} character`,
    other: `${formatNumber(limit)} characters`,
  });
}

function videos(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} video`,
    other: `${formatNumber(count)} videos`,
  });
}

const errorCodes = {
  not_found: "It wasn't found.",
  invalid_request: "The request wasn't valid.",
  internal: "Something went wrong on the server.",
  forbidden: "This action isn't allowed.",
  conflict: "Something changed in the meantime. Reload and try again.",
  invalid_media_directory: "That folder can't be used as a media folder.",
  unsupported_media_directory: "That kind of folder isn't supported as a media folder.",
  media_folder_not_found: "The media folder wasn't found.",
  overlapping_media_directories:
    "That folder overlaps a media folder that's already added.",
  scan_in_progress: "A scan is in progress. Try again when it finishes.",
  media_folders_not_configured: "Add a media folder first.",
  directory_unavailable: "That folder can't be opened.",
  probe_not_failed: "This video doesn't need to be read again.",
  open_unavailable: "The file couldn't be opened.",
  file_missing: "The video file is missing.",
  tag_not_found: "The tag wasn't found.",
  tag_name_taken: ({ tagName }) =>
    tagName === undefined
      ? "That name is already used by another tag."
      : `That name is already used by the tag "${tagName}".`,
  tag_merge_required: ({ tagName }) =>
    tagName === undefined
      ? "Another tag already has that name. Merge the tags instead."
      : `The tag "${tagName}" already has that name. Merge the tags instead.`,
  unauthenticated: "Sign in to continue.",
  invalid_credentials: "The username or password is incorrect.",
  login_throttled: "Too many sign-in attempts. Wait a moment and try again.",
  account_already_configured: "The account is already set up.",
} satisfies Record<ErrorCode, ErrorEntry>;

const errorReasons = {
  name_is_tag: ({ tagName }) =>
    tagName === undefined
      ? "A tag with that name already exists."
      : `A tag named "${tagName}" already exists.`,
  name_is_synonym: ({ tagName }) =>
    tagName === undefined
      ? "That name is already a synonym of another tag."
      : `That name is already a synonym of the tag "${tagName}".`,
  username_length: ({ limit }) =>
    limit === undefined
      ? "The username is too long."
      : `Use a username of 1 to ${characters(limit)}.`,
  password_length: ({ limit }) =>
    limit === undefined
      ? "The password is too long."
      : selectPlural(limit, {
          one: `Use a password of at most ${formatNumber(limit)} byte.`,
          other: `Use a password of at most ${formatNumber(limit)} bytes.`,
        }),
  tag_name_empty: "Enter a tag name.",
  tag_name_control_characters:
    "Tag names can't contain line breaks, tabs or other control characters.",
  tag_name_too_long: ({ limit }) =>
    limit === undefined
      ? "The tag name is too long."
      : `Use a tag name of ${characters(limit)} or fewer.`,
  merge_same_tag: "A tag can't be merged into itself.",
  search_too_long: ({ limit }) =>
    limit === undefined
      ? "The search is too long."
      : `Use a search of ${characters(limit)} or fewer.`,
  too_many_tag_filters: ({ limit }) =>
    limit === undefined
      ? "Too many tags are selected as filters."
      : `Filter by at most ${formatNumber(limit)} tags.`,
  too_many_videos: ({ limit }) =>
    limit === undefined
      ? "Select fewer videos."
      : `Select between 1 and ${videos(limit)}.`,
  guest_filter_not_allowed:
    "Sign in to filter by watch status or tags, or to sort by last played.",
  invalid_cursor: "The list changed while loading. Reload it.",
  invalid_folder_path: "That folder path isn't valid.",
  relative_directory_path: "The path must be absolute.",
  video_not_found: "The video wasn't found.",
  folder_not_found: "The folder wasn't found.",
  not_folder_group: "This folder isn't a group.",
  no_scan: "No scan has run yet.",
  directory_not_found: "That folder wasn't found.",
  file_unavailable: "The video file can't be opened.",
  media_folders_changed:
    "The media folders were changed elsewhere. Reload and try again.",
  root_group_not_taggable: "A media folder's own group can't be turned into a tag.",
  folder_not_group: "This folder is no longer a group.",
  probe_info_missing: "This video is missing the information needed for this.",
  seek_preview_generating: "The seek preview is still being generated.",
  transcode_unavailable: "This video can't be converted for playback.",
  cross_origin: "This change must be made from vv itself.",
  open_not_local: "Files can only be opened on the computer running vv.",
} satisfies Record<ErrorReason, ErrorEntry>;

const probeErrors = {
  file_unavailable: "The video file couldn't be read.",
  probe_unavailable: "ffprobe couldn't be started, so the video couldn't be read.",
  probe_failed: "The file is damaged or isn't a supported video.",
  invalid_metadata: "The video's information couldn't be understood.",
  internal: "Something went wrong while reading the video.",
} satisfies Record<ProbeErrorCode, string>;

const scanErrors = {
  media_folder_unreadable: (path?: string) =>
    path === undefined
      ? "A media folder couldn't be read."
      : `The media folder couldn't be read: ${path}`,
  media_folder_not_directory: (path?: string) =>
    path === undefined
      ? "A media folder isn't a folder."
      : `The media folder isn't a folder: ${path}`,
  location_unreadable: (path?: string) =>
    path === undefined
      ? "A location in a media folder couldn't be read."
      : `This location couldn't be read: ${path}`,
  interrupted: () => "The scan was interrupted.",
  internal: () => "Something went wrong during the scan.",
} satisfies Record<ScanErrorCode, (path?: string) => string>;

export const en = {
  common: {
    close: "Close",
    reload: "Reload",
  },
  app: {
    routeLoadFailed: {
      title: "Couldn't load this page",
      description: "Reload the page to try again.",
    },
  },
  time: {
    justNow: "just now",
  },
  count: {
    videos,
  },
  video: {
    unplayable: {
      failed: "Couldn't read this video",
      pending: "Checking…",
      missingInfo: "Missing information needed for playback",
    },
  },
  list: {
    inconsistentPage:
      "The list keeps changing, so it couldn't be loaded. Try again in a moment.",
  },
  folderGrouping: {
    ungrouped: (name: string) => `Ungrouped "${name}"`,
    tagged: (name: string, created: boolean) =>
      created
        ? `Created the tag "${name}" and ungrouped`
        : `Added the tag "${name}" and ungrouped`,
    cannotTag: (name: string) =>
      `"${name}" can't be used as a tag name, so it can't become a tag`,
    notGroup: "This folder is no longer a group",
    changeFailed: "Couldn't make the change",
  },
  tagName: {
    required: "Enter a name",
    controlCharacters: "Line breaks and tabs aren't allowed",
    tooLong: (limit: number, length: number) =>
      `Use ${characters(limit)} or fewer (currently ${formatNumber(length)})`,
  },
  errors: {
    code: errorCodes,
    reason: errorReasons,
    requestFailed: (status: number) => `Request failed (HTTP ${String(status)})`,
    unreachable: "Couldn't reach the server. Check that vv is running and try again.",
    unexpected: "Something went wrong.",
    probe: probeErrors,
    probeUnknown: "Couldn't read this video's information.",
    scan: scanErrors,
    scanUnknown: "The scan failed.",
  },
};
