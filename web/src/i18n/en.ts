import type { components } from "../api/gen/openapi";
import { formatList, formatNumber, selectPlural } from "./intl";

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
type ScanActivityKind = components["schemas"]["ScanActivityKind"];
type ScanIssueKind = components["schemas"]["ScanIssueKind"];
type VideoEncoderChoice = components["schemas"]["VideoEncoderChoice"];
type EncoderUnavailableReason = components["schemas"]["EncoderUnavailableReason"];

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

function items(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} item`,
    other: `${formatNumber(count)} items`,
  });
}

function seconds(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} second`,
    other: `${formatNumber(count)} seconds`,
  });
}

function folders(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} folder`,
    other: `${formatNumber(count)} folders`,
  });
}

function tagCount(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} tag`,
    other: `${formatNumber(count)} tags`,
  });
}

/** selectedTags は「The 8 selected tags」（1 件は「The selected tag」）である。 */
function selectedTags(count: number): string {
  return selectPlural(count, {
    one: "The selected tag",
    other: `The ${formatNumber(count)} selected tags`,
  });
}

/** someOf は「8 of the 12 selected tags are」（働く数が 1 なら「is」）である。 */
function someOf(applied: number, selected: number): string {
  return `${formatNumber(applied)} of the ${formatNumber(selected)} selected tags ${applied === 1 ? "is" : "are"}`;
}

function pronoun(count: number): string {
  return count === 1 ? "It" : "They";
}

/**
 * removal はまとめての却下・削除の確認の動詞句である。count はその主語のタグの数、
 * videoCount は影響を受ける動画の本数で、0 なら「aren't on any videos」の形にする。
 */
function removal(count: number, videoCount: number): string {
  if (videoCount === 0)
    return count === 1 ? "isn't on any videos" : "aren't on any videos";
  return `will be removed from ${videos(videoCount)}`;
}

/** leftAsIs は「The 4 confirmed tags are left as they are.」である。 */
function leftAsIs(count: number, kind: "confirmed" | "tentative"): string {
  return selectPlural(count, {
    one: `The ${kind} tag is left as it is.`,
    other: `The ${formatNumber(count)} ${kind} tags are left as they are.`,
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
  tag_not_tentative: "The tag is already confirmed.",
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
  too_many_tags: ({ limit }) =>
    limit === undefined
      ? "Select fewer tags."
      : `Select between 1 and ${tagCount(limit)}.`,
  guest_filter_not_allowed:
    "Sign in to filter by watch status, tags, or favorites, or to sort by last played or date favorited.",
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
  encoder_unavailable: "That video encoder isn't available on this server.",
  api_token_name_empty: "Enter a name for the token.",
  api_token_name_control_characters: "The name can't contain control characters.",
  api_token_name_too_long: ({ limit }) =>
    limit === undefined
      ? "The name is too long."
      : `The name can be up to ${characters(limit)}.`,
  subtitle_unavailable: "The subtitle file can't be read.",
  display_name_control_characters: "The name can't contain control characters.",
  display_name_too_long: ({ limit }) =>
    limit === undefined
      ? "The name is too long."
      : `The name can't be longer than ${characters(limit)}.`,
  duration_unknown: "The video's length isn't known yet.",
  thumbnail_position_out_of_range: "The position is past the end of the video.",
  thumbnail_frame_unavailable: "No image could be made from this frame.",
  too_few_videos: "Select at least two videos.",
  representative_not_selected: "Pick which video to show in the library.",
  not_bundled: "This video isn't bundled with others.",
  listen_failed: "vv couldn't change who can connect, so the previous setting is kept.",
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
    appName: "VVMDM",
    brandHome: "VVMDM home",
    close: "Close",
    reload: "Reload",
    retry: "Retry",
    cancel: "Cancel",
    back: "Back",
  },
  designSystem: {
    title: "Design system",
    foundations: "Foundations",
    components: "Components",
    patterns: "Page patterns",
    foundation: {
      surfaces: "Surfaces, darkest first",
      colours: "Colour roles",
      type: "Type",
      spacing: "Spacing and sizes",
      namedSizes: "Named sizes",
      radius: "Radius",
      shadow: "Shadow",
      sample: "Kyoto Arashiyama, morning walk 1080p 49:10",
      onSurface: (surface: string) => `Text on ${surface}`,
    },
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
    loading: "Loading…",
    resultCount: videos,
    noMatches: "No videos match these conditions",
    loadFailed: "Couldn't load the list",
    loadMoreFailed: (reason: string) => `Couldn't load more: ${reason}`,
    noMatchesHint: "Try a different search or change the filters.",
    changeSearch: "Change search",
    guestEmpty: {
      title: "No videos are public",
      description: "Sign in to see all videos.",
      signIn: "Sign in",
    },
    scan: "Scan",
    scanning: "Scanning…",
    cardSize: "Card size",
    viewAndSort: "View and sort",
    search: {
      label: "Search videos",
      placeholder: "Search",
      clear: "Clear search",
    },
    searchHelp: {
      title: "How to search",
      allWords: {
        example: "kyoto 2024",
        meaning: "Finds videos that contain every space-separated word",
      },
      phrase: {
        example: '"kyoto trip 2024"',
        before: "Words wrapped in ",
        after: " are searched for as one term, spaces included",
      },
      exclude: {
        example: "kyoto -2023",
        before: "Prefix a word with ",
        after: " to leave out videos that contain it",
      },
      either: {
        example: "kyoto OR nara",
        alternative: "kyoto | nara",
        meaning: "Finds videos that contain either word. Binds tighter than a space",
      },
      normalization:
        "Full-width and half-width characters, upper and lower case, and hiragana and katakana are treated the same.",
      termLimit: (limit: number) =>
        `Only the first ${formatNumber(limit)} terms are used.`,
    },
    filter: {
      label: "Filter",
      applied: (count: number) => `Filter (${formatNumber(count)} applied)`,
      watch: "Watch status",
      watchOptions: {
        all: "All",
        unwatched: "Unwatched",
        inProgress: "In progress",
        watched: "Watched",
      },
      playableOnly: "Playable only",
      favoritesOnly: "Favorites only",
      clear: "Clear filters",
    },
    sort: {
      heading: "Sort by",
      current: (label: string) => `Sort by: ${label}`,
      kinds: {
        added: "Date added",
        modified: "Date modified",
        created: "Date created",
        title: "Title",
        duration: "Length",
        size: "File size",
        played: "Recently played",
        favorited: "Date favorited",
        random: "Random",
      },
      wording: {
        added: { asc: "oldest first", desc: "newest first" },
        modified: { asc: "oldest first", desc: "newest first" },
        created: { asc: "oldest first", desc: "newest first" },
        duration: { asc: "shortest first", desc: "longest first" },
        size: { asc: "smallest first", desc: "largest first" },
        played: { asc: "least recently played", desc: "most recently played" },
        favorited: { asc: "oldest first", desc: "newest first" },
      },
      asc: "Ascending",
      desc: "Descending",
      withWording: (direction: string, wording: string) => `${direction} (${wording})`,
      toAsc: (current: string) => `${current}. Press for ascending`,
      toDesc: (current: string) => `${current}. Press for descending`,
      direction: "Sort direction",
      shuffle: "Shuffle",
    },
    card: {
      select: (title: string) => `Select "${title}"`,
      withLocation: (title: string, location: string) => `${title}, ${location}`,
      public: "Public",
      // お気に入りの付け外しの読み上げ名と、変えられなかったときのトースト
      // （specs/035-favorites/ui-design.md「Words」）。
      favorite: (title: string) => `Favorite "${title}"`,
      favoriteFailed: (reason: string) => `Couldn't change the favorite: ${reason}`,
      watchedRatio: "Watched portion",
      // 帯にいる間の時刻の表示。位置と長さは formatDuration で整えた文字列
      // （specs/032-card-scrub-preview/ui-design.md「Words」）。
      scrubTime: (position: string, duration: string) => `${position} / ${duration}`,
      watched: "Watched",
      noImage: "No image",
      preparing: "Preparing",
    },
  },
  library: {
    title: "Library",
    resultsLabel: "Search results",
    // GET /api/library の total は項目（カード）の数で、グループのカードも 1 つと数える。動画の本数ではない。
    resultCount: items,
    empty: {
      title: "No videos yet",
      description: "Put videos in a media folder and scan to see them here.",
    },
    view: {
      label: "View",
      compact: "View (compact)",
      grid: "Grid",
      list: "List",
    },
    columns: {
      title: "Title",
      duration: "Length",
      quality: "Quality",
      size: "Size",
      added: "Added",
    },
    deletedTagsRemoved: "Removed deleted tags from the filter",
    selectAllFailed: (reason: string) => `Couldn't select everything: ${reason}`,
    activeTags: {
      label: "Filtering by tags",
      list: "Tags in the filter",
      remove: "Remove the tag filter",
      removeNamed: (name: string) => `Remove the filter for ${name}`,
    },
    tagRow: {
      label: "Tags",
      fromFolder: "(from the folder name)",
      filterBy: (name: string) => `Filter by ${name}`,
      filterByFromFolder: (name: string) => `Filter by ${name} (from the folder name)`,
      tentative: "Tentative",
      filterByTentative: (name: string) => `Filter by ${name} (tentative)`,
      filterByFromFolderTentative: (name: string) =>
        `Filter by ${name} (from the folder name, tentative)`,
      more: (count: number) => `+${formatNumber(count)}`,
      showMore: (count: number) =>
        selectPlural(count, {
          one: `Show ${formatNumber(count)} more tag`,
          other: `Show ${formatNumber(count)} more tags`,
        }),
    },
    group: {
      count: videos,
      select: (name: string) => `Select the group "${name}"`,
      favorite: (name: string) => `Favorite group "${name}"`,
      label: (name: string, count: number, watched: number) =>
        watched >= 1
          ? `${name}, group of ${videos(count)}, ${formatNumber(watched)} watched`
          : `${name}, group of ${videos(count)}`,
      watchedRatio: "Share of videos watched",
      progress: (watched: number, total: number) =>
        `${formatNumber(watched)} / ${formatNumber(total)}`,
    },
    selection: {
      region: "Selection actions",
      count: (count: number) => `${videos(count)} selected`,
      selectAll: "Select all",
      selectingAll: "Selecting…",
      clear: "Clear selection (Esc)",
      addTag: "Add tag",
      removeTag: "Remove tag",
      create: (name: string) => `Create "${name}"`,
      synonym: (synonym: string) => `Synonym: ${synonym}`,
      videoCount: videos,
      partial: (count: number, total: number) =>
        `Some: ${formatNumber(count)} / ${formatNumber(total)}`,
      partialLabel: (name: string, count: number, total: number) =>
        `${name}, only some videos, ${formatNumber(count)} of ${formatNumber(total)}`,
      added: (count: number, name: string) => `Added "${name}" to ${videos(count)}`,
      removed: (count: number, name: string) => `Removed "${name}" from ${videos(count)}`,
      tagGone: (name: string) =>
        `The tag "${name}" no longer exists, so the tags were reloaded`,
      addFailed: (reason: string) => `Couldn't add the tag: ${reason}`,
      removeFailed: (reason: string) => `Couldn't remove the tag: ${reason}`,
      loading: "Loading…",
      summaryFailed: "Couldn't load the tags",
      nothingToRemove: "The selected videos have no tags that can be removed",
      favorite: "Favorite",
      addFavorites: "Add to favorites",
      removeFavorites: "Remove from favorites",
      favoritesAdded: (count: number) => `Added ${items(count)} to favorites`,
      favoritesRemoved: (count: number) => `Removed ${items(count)} from favorites`,
      favoritesFailed: (reason: string) => `Couldn't change the favorites: ${reason}`,
      visibility: "Visibility",
      makePublic: "Make public",
      makePrivate: "Make private",
      madePublic: (count: number) => `Made ${videos(count)} public`,
      madePrivate: (count: number) => `Made ${videos(count)} private`,
      visibilityFailed: (reason: string) => `Couldn't change the visibility: ${reason}`,
      bundle: "Bundle as versions",
      bundleOverLimit: (limit: number) => `Bundle up to ${videos(limit)} at a time`,
    },
  },
  folders: {
    title: "Folders",
    breadcrumbs: "Breadcrumbs",
    mediaFolders: "Media folders",
    subfolders: "Folders",
    videos: "Videos",
    sectionCount: (count: number) => formatNumber(count),
    directVideos: (count: number) => `${videos(count)} directly in this folder`,
    searchResults: "Search results",
    thisFolder: "This folder",
    searchIn: (name: string) => `Search in ${name}`,
    searchInPlaceholder: "Search this folder",
    searchAll: "Search videos in all folders",
    searchAllPlaceholder: "Search all folders",
    searchingAll: "Searching all folders",
    searchingInside: "— searching inside",
    card: {
      videos,
      folders,
      label: (name: string, videoCount: number, folderCount: number) =>
        `${name}, ${videos(videoCount)}, ${folders(folderCount)}`,
      labelWithPath: (label: string, path: string) => `${label}, ${path}`,
    },
    empty: {
      title: "No videos in this folder yet",
      rootTitle: "The media folders are empty",
      description: "No videos or subfolders were found. Put files there, then scan.",
    },
    noMediaFolders: {
      title: "No media folders yet",
      description: "Add a media folder in Settings and scan to see it here.",
      openSettings: "Open Settings",
    },
    notFound: {
      title: "This folder wasn't found",
      description: "It was removed from the media folders, or its videos are gone.",
      back: "Go to all folders",
    },
    grouping: {
      heading: "Grouping in the library",
      modes: {
        auto: "Automatic",
        ungroup: "Don't group",
        groupDirect: "Group this folder's videos",
      },
      grouped: "1 item in the library",
      ungrouped: "One by one in the library",
      trigger: (grouped: boolean) =>
        grouped
          ? "Grouping in the library: shown as 1 item. Open the menu"
          : "Grouping in the library: shown one by one. Open the menu",
      changing: "Changing the grouping in the library",
      changingShort: "Changing…",
      autoHint:
        "Automatic: groups folders below a media folder that have no subfolders and 2 or more videos",
      toTag: "Turn the group into a tag",
    },
  },
  tags: {
    title: "Tags",
    newTag: "New tag",
    loading: "Loading…",
    count: tagCount,
    filteredCount: (shown: number, total: number) =>
      `${formatNumber(shown)} of ${tagCount(total)}`,
    // 見出しの下のタブ（specs/036-tag-admin-scale/ui-design.md「Tabs」）。
    tabs: {
      label: "Tag lists",
      tags: "Tags",
    },
    // 一覧の上の列の見出し（ui-design.md「Column header」）。
    columns: {
      name: "Name",
      videos: "Videos",
    },
    // 見出しの下の、効いている絞り込みのチップ（ui-design.md「Active filters」）。
    activeFilters: {
      label: "Active filters",
      remove: (name: string) => `Remove the filter "${name}"`,
    },
    loadFailed: "Couldn't load the tags",
    // 一覧を持ったまま先頭のページを読めなかったとき（ui-design.md「Stale list」）。
    staleList: (reason: string) =>
      `Couldn't load tags: ${reason}. The list below may not match the current search, filters and sort.`,
    // 一覧の末尾の続きの状態（ui-design.md「Loading more」）。
    loadingMore: "Loading more tags…",
    loadMoreFailed: (reason: string) => `Couldn't load more: ${reason}`,
    listChanged:
      "Tags were added or removed elsewhere, so the rest of this list may be out of date.",
    reloadList: "Reload",
    empty: {
      title: "No tags yet",
      description:
        "Add tags from a video's playback page or the library's selection bar. You can also create them here first.",
    },
    noMatches: (query: string) => `No tags match "${query}"`,
    clearSearch: "Show all tags",
    // 仮のタグ（specs/031-tentative-tags/ui-design.md「Words」）。
    tentative: "Tentative",
    tentativeOnly: "Tentative only",
    tentativeOnlyHint: "Show only tags created by automatic tagging",
    noTentative: {
      title: "No tentative tags",
      description:
        "Tags created by automatic tagging appear here until you confirm or reject them.",
    },
    noTentativeMatches: (query: string) => `No tentative tags match "${query}"`,
    // 0 本のタグの絞り込みと並び順（specs/036-tag-admin-scale/ui-design.md「Words」）。
    // 0 本のタグは「Unused」と呼び、「empty」「orphan」は使わない。
    unusedOnly: "Unused only",
    unusedOnlyHint: "Show only tags that aren't on any videos",
    noUnused: {
      title: "No unused tags",
      description: "Every tag is on at least one video.",
    },
    noUnusedTentative: "No unused tentative tags",
    noUnusedMatches: (query: string) => `No unused tags match "${query}"`,
    noUnusedTentativeMatches: (query: string) =>
      `No unused tentative tags match "${query}"`,
    sort: {
      heading: "Sort by",
      current: (label: string) => `Sort by: ${label}`,
      compact: "Sort",
      direction: "Sort direction",
      kinds: {
        name: "Name",
        count: "Video count",
        created: "Date created",
      },
      toggle: {
        countDesc: "Descending (most videos first). Press for ascending",
        countAsc: "Ascending (fewest videos first). Press for descending",
        createdDesc: "Descending (newest first). Press for ascending",
        createdAsc: "Ascending (oldest first). Press for descending",
      },
      segments: {
        countDesc: "Most videos first",
        countAsc: "Fewest videos first",
        createdDesc: "Newest first",
        createdAsc: "Oldest first",
      },
    },
    confirmed: (name: string) => `Confirmed "${name}"`,
    rejected: (name: string) => `Rejected "${name}"`,
    alreadyConfirmed: "This tag was already confirmed, so the list was reloaded",
    rejectedNames: {
      heading: "Rejected names",
      description:
        "Automatic tagging won't create these names. Allow a name again to let it be created.",
      empty: "No rejected names",
      allow: (name: string) => `Allow "${name}" again`,
      allowShort: "Allow again",
      loadFailed: "Couldn't load the rejected names",
      loadMoreFailed: "Couldn't load more rejected names",
      removeFailed: (name: string, reason: string) =>
        `Couldn't remove "${name}": ${reason}`,
    },
    search: {
      label: "Search tags",
      placeholder: "Search tags",
      clear: "Clear search",
    },
    gone: "This tag no longer exists, so the list was reloaded",
    deleted: (name: string) => `Deleted "${name}"`,
    merged: (source: string, target: string) => `Merged "${source}" into "${target}"`,
    // タグ名の競合の説明。送った名前と、API の tagName（競合先のタグの元の名前）を合わせる
    // （contracts/error-api.md §1）。
    nameIsTag: (tagName: string) => `A tag named "${tagName}" already exists.`,
    nameIsOwnName: (tagName: string) => `"${tagName}" is already this tag's name.`,
    nameIsSynonym: (submitted: string, tagName: string) =>
      `"${submitted}" is already a synonym of the tag "${tagName}".`,
    nameIsOwnSynonym: (submitted: string) =>
      `"${submitted}" is already a synonym of this tag.`,
    create: {
      label: "New tag name",
      placeholder: "Tag name",
      submit: "Create",
      submitting: "Creating…",
    },
    row: {
      open: (name: string) => `Open the library filtered by ${name}`,
      synonyms: (list: string) => `Synonyms: ${list}`,
      videoCount: videos,
      renameLabel: (name: string) => `New name for "${name}"`,
      rename: "Rename",
      synonymsButton: "Synonyms",
      more: "More actions",
      merge: "Merge into another tag…",
      delete: "Delete…",
      confirm: "Confirm",
      reject: "Reject…",
      select: (name: string) => `Select "${name}"`,
      actions: "Actions",
    },
    // 行の選択とまとめての確定・却下・削除（specs/036-tag-admin-scale/ui-design.md「Words」）。
    selectAllLoaded: (count: number) =>
      selectPlural(count, {
        one: `Select the ${formatNumber(count)} loaded tag`,
        other: `Select all ${formatNumber(count)} loaded tags`,
      }),
    selectAllOverLimit: (limit: number) =>
      `Too many tags are loaded to select them all at once (limit ${formatNumber(limit)}). Narrow the list with search or a filter.`,
    clearSelection: "Clear selection",
    selection: {
      region: "Selected tags",
      count: (count: number) => `${tagCount(count)} selected`,
      confirm: "Confirm",
      more: "More",
      clear: "Clear selection",
      mergeInto: "Merge into one tag…",
      reject: "Reject…",
      delete: "Delete…",
      noTentative: "No tentative tags are selected",
      noConfirmed: "No confirmed tags are selected",
      overLimit: (limit: number) =>
        `Too many tags are selected to act on them together (limit ${formatNumber(limit)}). Clear some of the selection.`,
      confirmed: (applied: number, skipped: number) =>
        skipped === 0
          ? `Confirmed ${tagCount(applied)}`
          : `Confirmed ${tagCount(applied)}. ${formatNumber(skipped)} ${skipped === 1 ? "was" : "were"} already confirmed.`,
      rejected: (applied: number, skipped: number) =>
        skipped === 0
          ? `Rejected ${tagCount(applied)}`
          : `Rejected ${tagCount(applied)}. ${selectPlural(skipped, {
              one: `${formatNumber(skipped)} confirmed tag was skipped.`,
              other: `${formatNumber(skipped)} confirmed tags were skipped.`,
            })}`,
      deleted: (applied: number, skipped: number) =>
        skipped === 0
          ? `Deleted ${tagCount(applied)}`
          : `Deleted ${tagCount(applied)}. ${selectPlural(skipped, {
              one: `${formatNumber(skipped)} tentative tag was skipped.`,
              other: `${formatNumber(skipped)} tentative tags were skipped.`,
            })}`,
      stale: "Some of the tags no longer existed, so the list was reloaded",
      merged: (count: number, target: string) =>
        `Merged ${tagCount(count)} into "${target}"`,
    },
    bulkDialog: {
      rejectTitle: "Reject selected tags",
      deleteTitle: "Delete selected tags",
      counting: "Counting the affected videos…",
      countFailed: (reason: string) => `Couldn't count the affected videos: ${reason}`,
      // 選んだすべてに働くとき。videoCount が 0 なら「removed from 0 videos」とは言わない。
      rejectAll: (selected: number, videoCount: number) =>
        `${selectedTags(selected)} ${removal(selected, videoCount)}, and automatic tagging won't create ${selected === 1 ? "its name" : "their names"} again. You can allow a name again from Rejected names.`,
      rejectSome: (applied: number, selected: number, videoCount: number) =>
        `${someOf(applied, selected)} tentative. ${pronoun(applied)} ${removal(applied, videoCount)}, and automatic tagging won't create ${applied === 1 ? "its name" : "their names"} again. ${leftAsIs(selected - applied, "confirmed")} You can allow a name again from Rejected names.`,
      rejectNone: "None of the selected tags are tentative.",
      deleteAll: (selected: number, videoCount: number) =>
        `${selectedTags(selected)} ${removal(selected, videoCount)}. This can't be undone.`,
      deleteSome: (applied: number, selected: number, videoCount: number) =>
        `${someOf(applied, selected)} confirmed. ${pronoun(applied)} ${removal(applied, videoCount)}. This can't be undone. ${leftAsIs(selected - applied, "tentative")} Reject ${selected - applied === 1 ? "it" : "them"} instead.`,
      deleteNone: "None of the selected tags are confirmed.",
    },
    rejectDialog: {
      title: (name: string) => `Reject "${name}"`,
      unused: (name: string) =>
        `This tag isn't on any videos. Automatic tagging won't create "${name}" again. You can allow the name again from Rejected names.`,
      used: (name: string, count: number) =>
        `This tag will be removed from ${videos(count)}, and automatic tagging won't create "${name}" again. You can allow the name again from Rejected names.`,
      submit: "Reject",
      submitting: "Rejecting…",
    },
    deleteDialog: {
      title: (name: string) => `Delete "${name}"`,
      unused: "This tag isn't on any videos.",
      used: (count: number) =>
        `This tag will be removed from ${videos(count)}. This can't be undone.`,
      submit: "Delete",
      submitting: "Deleting…",
    },
    mergeDialog: {
      title: (name: string) => `Merge "${name}"`,
      target: "Tag to merge into",
      synonymHint: (synonym: string) => `Synonym: ${synonym}`,
      // 統合先の候補の一覧が空のとき（ui-design.md「Merge dialog」）。
      noCandidates: "No matching tags",
      // 窓の下端の「統合元 → 統合先」。統合元が複数なら数で言う。
      summarySources: (count: number) => tagCount(count),
      // 統合先の候補をサーバーで引けなかったとき（ui-design.md「Target candidates」）。
      searchFailed: (reason: string) => `Couldn't search tags: ${reason}`,
      videoCount: videos,
      warning: (source: string, count: number, target: string) =>
        `The ${videos(count)} tagged "${source}" get the tag "${target}". "${source}" and its synonyms become synonyms of "${target}", and "${source}" leaves the tag list. This can't be undone.`,
      // 統合元が複数のとき（specs/036-tag-admin-scale/ui-design.md「Words」）。count は統合先を
      // 外した統合元の数で、選んだ数ではない。videoCount が 0 なら「0 videos」とは言わない。
      titleMany: (count: number) => `Merge ${tagCount(count)}`,
      sources: "Tags to merge",
      kept: "kept",
      keptNote: (target: string, others: number) =>
        `"${target}" is kept and the other ${selectPlural(others, {
          one: "tag merges",
          other: `${formatNumber(others)} tags merge`,
        })} into it.`,
      onlyTarget: (target: string) =>
        `Choose another tag to merge into: "${target}" is the only tag selected.`,
      warningMany: (count: number, videoCount: number, target: string) =>
        `${
          videoCount === 0
            ? `These ${formatNumber(count)} tags aren't on any videos.`
            : `The ${videos(videoCount)} tagged with these ${formatNumber(count)} tags get the tag "${target}".`
        } Their names and synonyms become synonyms of "${target}", and the ${formatNumber(count)} tags leave the tag list. This can't be undone.`,
      submit: "Merge",
      submitting: "Merging…",
    },
    synonymsDialog: {
      title: (name: string) => `Synonyms of "${name}"`,
      list: "Synonyms",
      remove: (name: string) => `Remove the synonym "${name}"`,
      add: "Add synonym",
      submit: "Add",
      addFailed: "Couldn't add the synonym",
      mergeWarning: (
        source: string,
        count: number,
        target: string,
        hasSynonyms: boolean,
      ) =>
        `"${source}" is a tag on ${videos(count)}. Merging it into "${target}" adds "${target}" to those videos, and "${source}"${hasSynonyms ? " and its synonyms become synonyms" : " becomes a synonym"} of "${target}". "${source}" leaves the tag list.`,
      back: "Back",
      merge: "Merge",
    },
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
  player: {
    documentTitle: (title: string) => `${title} · VVMDM`,
    header: {
      home: "Home",
      folder: "Folder",
    },
    // 操作バー（video.js）の読み上げ名とツールチップ。キーボード操作を持つボタンには
    // withKey でキーを添える（specs/023-english-i18n/research.md R-9）。
    controls: {
      withKey: (label: string, key: string) => `${label} (${key})`,
      play: "Play",
      pause: "Pause",
      replay: "Replay",
      mute: "Mute",
      unmute: "Unmute",
      fullscreen: "Fullscreen",
      exitFullscreen: "Exit fullscreen",
      pictureInPicture: "Picture-in-picture",
      exitPictureInPicture: "Exit picture-in-picture",
      playbackRate: "Playback speed",
      currentTime: "Current time",
      duration: "Duration",
      progressBar: "Playback position",
      volumeLevel: "Volume",
      videoPlayer: "Video player",
      restart: "Restart",
      /** 操作バーのメニュー（再生速度・字幕）で選んでいる項目に、読み上げ用に添える文言。 */
      menuItemSelected: ", selected",
      transcoding: "Converting for playback",
      transcodingDetail:
        "The browser can't play this format directly, so it's converted while it plays. Seeking takes a few seconds.",
      // 画質の名前（480p など）は翻訳しない（specs/027-playback-quality/ui-design.md「Words」）。
      transcodingTo: (quality: string) => `Converting to ${quality}`,
      transcodingToDetail: (quality: string) =>
        `Playing a ${quality} version converted while it plays, so it needs less bandwidth. Choose “Original” in the quality menu to go back. Seeking takes a few seconds.`,
      // 操作バーの画質メニュー（ui-design.md「Words」「Control bar: quality menu」）。
      quality: "Quality",
      qualityName: (quality: string) => quality,
      qualityOriginal: "Original",
      qualityOriginalOf: (quality: string) => `Original (${quality})`,
      qualityOriginalShort: "Orig",
      qualityNoSmaller: "No smaller sizes for this video",
    },
    // 字幕ボタンとメニュー（specs/028-sidecar-subtitles research.md R-10）。
    subtitles: {
      button: "Subtitles",
      off: "Off",
      /** ラベルの無い字幕（`<名前>.srt`）の表示名。 */
      default: "Default",
    },
    neighbors: {
      previous: "Previous video",
      next: "Next video",
      withTitle: (label: string, title: string) => `${label}: ${title}`,
    },
    loading: "Loading",
    reconnecting: "Connection lost · Reconnecting",
    // 途切れの警告。知らせるだけで、画質を下げる勧めや切り替えは書かない
    // （specs/027-playback-quality/ui-design.md「Words」）。
    stallWarning: {
      message: "Slow connection is interrupting playback",
      dismiss: "Dismiss",
    },
    playbackFailed: {
      network: {
        title: "Couldn't reach the server",
        description:
          "Playback stopped because the connection to the server was lost. Check your network and try again.",
      },
      decode: {
        title: "Couldn't play this video",
        description: "The video data couldn't be read. The file may be damaged.",
      },
      source: {
        title: "Couldn't play this video",
        description:
          "The file may have been moved or deleted, or the browser may not support its format.",
      },
      retryFrom: (time: string) => `Try again from ${time}`,
    },
    stages: {
      title: "Getting ready to play",
      description:
        "Reading the video's information. When that's done, you can play it right here.",
      note: "Playback starts once “Reading video information” is done. The rest is created during playback.",
      names: {
        detect: "Finding the file",
        probe: "Reading video information",
        thumbnail: "Thumbnail",
        seekPreview: "Seek preview",
        preview: "List preview",
      },
      states: {
        done: "Done",
        active: "In progress",
        waiting: "Waiting",
        failed: "Couldn't create",
      },
    },
    creating: {
      thumbnail: "the thumbnail",
      seekPreview: "the seek preview",
      preview: "the list preview",
      line: (parts: readonly string[]) =>
        `Creating ${formatList(parts)} · You can play the video now`,
    },
    readFailure: {
      title: "Couldn't read this video",
      reprobe: "Read again",
      openFile: "Open file",
      reprobeFailed: "Couldn't start reading the video again",
    },
    missing: {
      title: "This video can't be opened",
      description: "It was removed from the library, or its file is gone.",
    },
    loadFailed: "Couldn't load this video",
    unplayable: {
      title: "This video can't be played",
      description: "It's missing the length or picture information needed for playback.",
    },
    ended: {
      announcement: "Playback finished",
      next: "Next video",
      playNext: "Play next",
      replay: "Watch again",
    },
    autoplay: {
      heading: "Up next",
      countdown: (remaining: number) => `in ${seconds(remaining)}`,
      announcement: (title: string, wait: number) =>
        `Playback finished. The next video, "${title}", plays in ${seconds(wait)}`,
      cancel: "Cancel",
      playNow: "Play now",
    },
    related: {
      heading: "Related videos",
      loadFailed: "Couldn't load related videos",
      group: "Up next",
      position: (position: number, count: number) =>
        `${formatNumber(position)} / ${formatNumber(count)}`,
      videoLink: (title: string, duration: string) => `${title} ${duration}`,
      watchedLink: (label: string) => `${label}, watched`,
      nowPlaying: "Now playing",
      watched: "Watched",
    },
    facts: {
      label: "File details",
      duration: "Length",
      size: "Size",
      added: "Added",
      edited: "Edited",
      created: "Created",
      /** 日付の項目の `title` と吹き出しの文字（例: Edited Sep 27, 2026, 3:04 PM）。 */
      dateDetail: (name: string, dateTime: string) => `${name} ${dateTime}`,
      openFile: "Open file",
      copyPath: "Copy path",
      pathCopied: "Copied the path",
      copyFailed: "Couldn't copy the path",
      openFailed: (reason: string) => `Couldn't open the file: ${reason}`,
      thumbnailAt: (time: string) => `Thumbnail at ${time}`,
      useCurrentFrame: "Use current frame as thumbnail",
      useAutomaticThumbnail: "Use automatic thumbnail",
      thumbnailFailed: (reason: string) => `Couldn't change the thumbnail: ${reason}`,
      // 右端の一群の先頭のお気に入りの付け外しと、変えられなかった 1 行
      // （specs/035-favorites/ui-design.md「Words」「Video page」）。
      favorite: "Favorite",
      favoriteFailed: (reason: string) => `Couldn't change the favorite: ${reason}`,
      technical: "Technical details",
      technicalPending: "Reading technical details…",
      technicalFailed: "Couldn't read the technical details",
    },
    versions: {
      count: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} version`,
          other: `${formatNumber(count)} versions`,
        }),
      show: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} version of this video. Show versions`,
          other: `${formatNumber(count)} versions of this video. Show versions`,
        }),
      label: "Versions",
      representative: "Representative",
      nowPlaying: "Now playing",
      play: (title: string, details: string) =>
        details === "" ? `Play ${title}` : `Play ${title}, ${details}`,
      more: (title: string) => `More actions for ${title}`,
      makeRepresentative: "Make representative",
      remove: "Remove from versions",
      loadFailed: "Couldn't load the versions",
      retry: "Retry",
      changeFailed: (reason: string) => `Couldn't change the versions: ${reason}`,
      removed: (title: string) => `Removed "${title}" from the versions`,
    },
    title: {
      edit: "Edit name",
      input: "Display name",
      save: "Save",
      cancel: "Cancel",
      fileName: "File name",
      fileNameTitle: (name: string) => `File name: ${name}`,
      saveFailed: (reason: string) => `Couldn't save the name: ${reason}`,
    },
    visibility: {
      label: "Show to people who aren't signed in",
      public: "Public",
      private: "Private",
      failed: (reason: string) => `Couldn't change the visibility: ${reason}`,
    },
    group: {
      menu: (name: string, position: number, count: number) =>
        `Group "${name}", video ${formatNumber(position)} of ${formatNumber(count)}. Grouping menu`,
      ungroup: "Ungroup",
      toTag: "Turn the group into a tag",
    },
    tags: {
      heading: "Tags",
      add: "Add tag",
      create: (name: string) => `Create "${name}"`,
      synonym: (synonym: string) => `Synonym: ${synonym}`,
      videoCount: videos,
      filterBy: (name: string) => `Filter by ${name}`,
      filterByFromFolder: (name: string) => `Filter by ${name} (from the folder name)`,
      filterByTentative: (name: string) => `Filter by ${name} (tentative)`,
      filterByFromFolderTentative: (name: string) =>
        `Filter by ${name} (from the folder name, tentative)`,
      remove: (name: string) => `Remove ${name} from this video`,
      gone: (name: string) =>
        `The tag "${name}" no longer exists, so the tags were reloaded`,
      attachFailed: (reason: string) => `Couldn't add the tag: ${reason}`,
      detachFailed: (reason: string) => `Couldn't remove the tag: ${reason}`,
    },
  },
  versions: {
    bundle: {
      title: "Bundle as versions",
      description: (count: number) =>
        `These ${formatNumber(count)} videos become versions of one video. Pick the one to show in the library. The library keeps that video's tags, position and visibility; the others' are set aside and come back if you remove them.`,
      representative: "Representative",
      row: (title: string, details: string) =>
        details === "" ? title : `${title}, ${details}`,
      alreadyBundled: (count: number) =>
        `Already ${formatNumber(count)} versions — all of them join`,
      submit: "Bundle",
      loadFailed: "Couldn't load the selected videos",
      failed: (reason: string) => `Couldn't bundle: ${reason}`,
      bundled: (count: number, title: string) =>
        `Bundled ${formatNumber(count)} videos as versions of "${title}"`,
    },
    duplicates: {
      documentTitle: "Duplicates",
      title: "Possible duplicates",
      loading: "Loading…",
      count: (shown: number, total: number) =>
        shown < total
          ? `Showing ${formatNumber(shown)} of ${formatNumber(total)} pairs`
          : selectPlural(total, {
              one: `${formatNumber(total)} pair`,
              other: `${formatNumber(total)} pairs`,
            }),
      reason: "Same length · similar frames",
      same: "Same video…",
      sameFor: (first: string, second: string) =>
        `Same video, for ${first} and ${second}`,
      different: "Different videos",
      differentFor: (first: string, second: string) =>
        `Different videos, for ${first} and ${second}`,
      dismissed: "Marked as different videos. They won't be suggested again",
      gone: "This pair is no longer a candidate",
      loadFailed: "Couldn't load the candidates",
      empty: {
        title: "No possible duplicates",
        description:
          "When a scan finds files that look like the same video, they show up here for you to confirm. You can also select videos in the library and bundle them yourself.",
      },
    },
  },
  tagName: {
    required: "Enter a name",
    controlCharacters: "Line breaks and tabs aren't allowed",
    tooLong: (limit: number, length: number) =>
      `Use ${characters(limit)} or fewer (currently ${formatNumber(length)})`,
  },
  auth: {
    gate: {
      unreachable: "Can't connect to the server",
      noResponse: "The server isn't responding. Check that it's running.",
    },
    fields: {
      username: "Username",
      password: "Password",
      confirmPassword: "Confirm password",
    },
    connectionWarning:
      "This connection isn't encrypted. Your username, password and sign-in status can be read in transit. Signing in doesn't protect against eavesdropping.",
    login: {
      title: "Sign in",
      description: "Sign in with the owner account",
      submit: "Sign in",
      submitting: "Signing in…",
      throttledFor: (wait: number) =>
        `Too many sign-in attempts. Try again in ${seconds(wait)}.`,
      failed: "Couldn't sign in. Try again.",
    },
    setup: {
      title: "Create an account",
      description:
        "Create the one account for this server. To change it later, use the server's command line.",
      submit: "Create account",
      submitting: "Creating the account…",
      goToLogin: "Go to sign in",
      usernameRequired: "Enter a username.",
      usernameRule: (limit: number) =>
        `Use a username of up to ${characters(limit)}, without leading or trailing spaces or control characters.`,
      passwordRequired: "Enter a password.",
      passwordMismatch: "The passwords don't match.",
      failed: "Couldn't finish setting up. Try again.",
    },
  },
  shell: {
    nav: {
      main: "Main navigation",
      account: "Account and settings",
      library: "Library",
      folders: "Folders",
      tags: "Tags",
      duplicates: "Duplicates",
      settings: "Settings",
      login: "Sign in",
      logout: "Sign out",
      loggingOut: "Signing out…",
      logoutFailed: "Couldn't sign out",
      menu: "Menu",
      closeMenu: "Close menu",
    },
    topBar: {
      scanning: "Scanning",
      refreshLibrary: "Refresh library",
      needsMediaFolder: "Add a media folder in Settings first",
      refreshing: "Refreshing",
      refresh: "Refresh",
    },
    scan: {
      startFailed: (reason: string) => `Couldn't start the scan: ${reason}`,
      notRun: "No scan has run yet",
      // 状態の言葉（specs/024-import-progress/ui-design.md「Words」）。走査と準備を
      // 言い分けない。
      status: {
        starting: "Starting",
        finding: "Scanning",
        running: "Scanning",
        done: "Done",
        partial: "Some failed",
        failed: "Scan failed",
      },
      // 進み具合。分母は変化のあったファイルと準備が残っていた動画で、済みには失敗して
      // 終わった動画も入る。
      videosDone: (settled: number, total: number) =>
        `${formatNumber(settled)} of ${videos(total)} done`,
      videosShort: (settled: number, total: number) =>
        `${formatNumber(settled)} / ${formatNumber(total)}`,
      denominator:
        "Counts changed files and videos that still needed preparing, not the whole library.",
      noChanges: "No changed files were found.",
      // 今の処理の行。動作の言葉を先に置き、長いファイル名は末尾を省略する。
      lookingForFiles: "Looking for files…",
      activity: {
        registering: "Adding to the library",
        probe: "Analyzing",
        thumbnail: "Creating the thumbnail",
        seekThumbnail: "Creating seek thumbnails",
        preview: "Creating the preview",
        fingerprint: "Checking for duplicates",
      } satisfies Record<ScanActivityKind, string>,
      activityLine: (action: string, fileName: string) => `${action} · ${fileName}`,
      /** 登録フォルダの表示名 / 相対パス / ファイル名。省略した行の全体を示す。 */
      location: (parts: readonly string[]) => parts.join(" / "),
      finishedAt: (when: string) => `Finished ${when}`,
      couldNotFinish: "The scan couldn't finish.",
      failedCount: (count: number) => `${formatNumber(count)} failed`,
      toCheckCount: (count: number) => `${formatNumber(count)} to check`,
      seeSettingsForList: "See Settings for the list.",
      // 読み上げは完了・一部失敗・失敗の節目だけにする。
      announce: {
        done: (toCheck: number) =>
          toCheck === 0
            ? "The scan is complete."
            : selectPlural(toCheck, {
                one: `The scan is complete. ${videos(toCheck)} is worth checking.`,
                other: `The scan is complete. ${videos(toCheck)} are worth checking.`,
              }),
        partial: (failed: number) =>
          `The scan finished with some failures. ${videos(failed)} may not be usable.`,
        failed: "The scan failed.",
      },
      /** 右下の本体の名前: 〈状態の言葉〉 M of N videos done, 〈問題の本数〉. */
      indicatorName: (status: string, progress: string | null, issues: string | null) =>
        `${progress === null ? status : `${status} ${progress}`}${issues === null ? "" : `, ${issues}`}`,
      issueCounts: (counts: readonly string[]) => formatList(counts),
      openStatus: (label: string) => `${label}. Open the scan status`,
      dismissResult: "Dismiss the scan result notice",
      progress: "Progress of the videos in this scan",
    },
  },
  settings: {
    title: "Settings",
    scanStatus: {
      heading: "Scan status",
      // 状態の言葉は shell.scan.status と同じものを使う。ここにはこの画面だけの状態を置く。
      state: {
        notRun: "Not run",
        fetchFailed: "Rechecking",
      },
      rechecking: "Rechecking the latest status",
      // 問題の一覧（specs/024-import-progress/ui-design.md「Issue List」）。
      issues: {
        heading: "Videos with problems",
        failed: "Failed",
        check: "Check",
        // 影響（その動画がどうなっているか）と理由。種類の漏れは型検査で見つかる
        // （research.md R-10）。
        kinds: {
          unreadable: {
            impact: "Not added to the library.",
            reason: "The file couldn't be read.",
          },
          changed_during_import: {
            impact: "Not added to the library.",
            reason: "The file changed during the scan.",
          },
          register_failed: {
            impact: "Not added to the library.",
            reason: "Saving it to the library failed.",
          },
          probe_failed: {
            impact: "It may not play.",
            reason: "It couldn't be analyzed as a video.",
          },
          thumbnail_failed: {
            impact: "It has no thumbnail.",
            reason: "The thumbnail couldn't be created.",
          },
          seek_thumbnail_failed: {
            impact: "No thumbnails appear while seeking.",
            reason: "The seek thumbnails couldn't be created.",
          },
          preview_failed: {
            impact: "No preview appears in the list.",
            reason: "The preview couldn't be created.",
          },
          fingerprint_failed: {
            impact: "It isn't checked for possible duplicates.",
            reason: "Its seek thumbnails couldn't be compared.",
          },
          thumbnail_first_frame: {
            impact: "The thumbnail uses the first frame instead.",
            reason: "The frame at the usual position couldn't be read.",
          },
          seek_thumbnail_full_decode: {
            impact: "The seek thumbnails were rebuilt from the whole video.",
            reason: "The usual method didn't work.",
          },
        } satisfies Record<ScanIssueKind, { impact: string; reason: string }>,
        impactAndReason: (impact: string, reason: string) => `${impact} ${reason}`,
        also: (impacts: readonly string[]) => `Also: ${impacts.join(" ")}`,
        /** 行のリンクの名前: ファイル名と目印と影響。 */
        openVideo: (fileName: string, marker: string, impact: string) =>
          `${fileName}. ${marker}: ${impact} Open the video`,
        showMore: "Show more",
        loadingMore: "Loading…",
        loadFailed: (reason: string) => `Couldn't load the list: ${reason}`,
      },
    },
    mediaFolders: {
      heading: "Media folders",
      description:
        "Folders on the server where VVMDM looks for videos. After a change, run a scan with “Refresh library” at the top. Scans don't start automatically.",
      lockedWhileScanning: "You can't change media folders while a scan is running",
      list: "Added media folders",
      loading: "Loading the media folders",
      loadFailed: (reason: string) => `Couldn't load the media folders: ${reason}`,
      empty: "No media folders yet",
      emptyHint: "Add a folder to start scanning manually.",
      removing: "Removing…",
      changing: "Changing…",
      current: "Current location",
      change: "Change folder",
      changeShort: "Change",
      remove: "Remove folder",
      removeShort: "Remove",
      add: "Add folder",
      addBlocked: "You can add a folder once the current folders have loaded",
      changedElsewhere:
        "This folder was changed or removed elsewhere. Check it and try again.",
      added: "Added. Run a scan to apply the change.",
      changed: "Changed. Run a scan to apply the change.",
      removed: "Removed. A scan doesn't start automatically.",
    },
    transcoding: {
      heading: "Video conversion",
      description:
        "How VVMDM encodes video when a browser can't play the original file. A hardware encoder needs the server's GPU and VVMDM running directly on the server; the Docker image uses software only.",
      guide: "How to set up hardware encoding",
      opensInNewTab: "(opens in a new tab)",
      loading: "Loading the video conversion settings",
      loadFailed: (reason: string) =>
        `Couldn't load the video conversion settings: ${reason}`,
      inUse: "In use now",
      checking: "Checking which encoders this server can use…",
      selectedUnavailable:
        "The selected encoder isn't available on this server, so videos are converted with software.",
      choices: "Video encoder",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't change the encoder: ${reason}`,
      encoder: {
        software: "Software",
        nvenc: "NVENC (NVIDIA)",
        qsv: "Quick Sync (Intel)",
        vaapi: "VAAPI (Intel/AMD)",
        videotoolbox: "VideoToolbox (macOS)",
        auto: "Automatic",
      } satisfies Record<VideoEncoderChoice, string>,
      state: {
        available: "Available",
        alwaysAvailable: "Always available",
        auto: "Uses an available hardware encoder, otherwise software",
        checking: "Checking…",
      },
      reason: {
        unsupported_os: "Not supported on this server's operating system",
        encoder_missing: "Not found on this server",
        check_failed: "The test encode failed",
        timed_out: "The test encode took too long",
      } satisfies Record<EncoderUnavailableReason, string>,
    },
    network: {
      heading: "Network",
      description:
        "By default only this PC can open VVMDM. Allow connections from your local network to open it from a phone or another computer on the same network.",
      loading: "Loading the network settings",
      loadFailed: (reason: string) => `Couldn't load the network settings: ${reason}`,
      lanAccess: "Allow connections from the local network",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't change the setting: ${reason}`,
      cautionHeading: "Before you turn this on",
      caution: {
        lan: "Any device on the same local network can open the sign-in page.",
        firewall: "If Windows Firewall asks, allow VVMDM on private networks.",
        http: "The connection uses HTTP, so passwords travel unencrypted. Use it only on a network you trust.",
      },
      addresses: "Open one of these addresses on the other device:",
      noAddresses:
        "This PC has no local network address right now. Connect it to a network and reload this page.",
    },
    apiTokens: {
      heading: "API tokens",
      description:
        "Tokens let external tools and MCP clients use this library as the owner. Anyone who has a token can read every video and change tags, so keep it as safe as your password.",
      guide: "How to use the API and MCP",
      opensInNewTab: "(opens in a new tab)",
      name: "Name",
      nameHint: "What the token is for, such as the tool that will use it.",
      create: "Create token",
      creating: "Creating…",
      createBlocked: "You can create a token once the current tokens have loaded",
      revealTitle: (name: string) => `Token for ${name}`,
      revealWarning:
        "This is the only time the token is shown. Copy it now. After you close this, it can't be shown again; if you lose it, revoke it and create a new one.",
      copy: "Copy token",
      copied: "Copied",
      copyFailed: "Couldn't copy. Select the token and copy it yourself.",
      done: "Done",
      list: "API tokens",
      loading: "Loading the API tokens",
      loadFailed: (reason: string) => `Couldn't load the API tokens: ${reason}`,
      created: (when: string) => `Created ${when}`,
      lastUsed: (when: string) => `Last used ${when}`,
      neverUsed: "Never used",
      revoke: "Revoke",
      revokeNamed: (name: string) => `Revoke ${name}`,
      revoked: "Revoked",
    },
    revokeTokenDialog: {
      title: "Revoke this token?",
      target: "Token",
      warning: "Tools using this token stop working right away. This can't be undone.",
      submit: "Revoke",
      revoking: "Revoking…",
    },
    removeDialog: {
      title: "Remove this folder?",
      target: "Media folder to remove",
      warning:
        "Videos that are only in this folder leave the list. Videos that are also in another media folder stay. Playback positions and watched status are kept. A scan doesn't start automatically.",
      submit: "Remove",
      removing: "Removing…",
    },
    picker: {
      addTitle: "Add media folder",
      changeTitle: "Change media folder",
      parent: "Go to the parent folder",
      fileSystem: "File system",
      list: "Folders",
      loading: "Loading the folders",
      toRoot: "Go to the top",
      noSubfolders: "No subfolders",
      selectFolder: "Select a folder",
      sameFolder: "This is the current folder",
      overlaps:
        "You can't select a media folder that's already added, or a folder inside or above one",
      addThis: "Add this folder",
      changeToThis: "Change to this folder",
      confirmTitle: "Change this folder?",
      before: "Before",
      after: "After",
      confirmWarning:
        "Videos that are only in the old folder leave the list. Videos that are also in another media folder stay. Playback positions and watched status are kept. A scan doesn't start automatically.",
      confirm: "Change",
      changing: "Changing…",
      adding: "Adding…",
    },
  },
  errors: {
    code: errorCodes,
    reason: errorReasons,
    requestFailed: (status: number) => `Request failed (HTTP ${String(status)})`,
    unreachable: "Couldn't reach the server. Check that VVMDM is running and try again.",
    unexpected: "Something went wrong.",
    probe: probeErrors,
    probeUnknown: "Couldn't read this video's information.",
    scan: scanErrors,
    scanUnknown: "The scan failed.",
  },
};
