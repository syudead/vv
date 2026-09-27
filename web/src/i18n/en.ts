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
    appName: "vv",
    close: "Close",
    reload: "Reload",
    retry: "Retry",
    cancel: "Cancel",
    back: "Back",
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
      clear: "Clear filters",
    },
    sort: {
      heading: "Sort by",
      current: (label: string) => `Sort by: ${label}`,
      kinds: {
        added: "Date added",
        modified: "Date modified",
        title: "Title",
        duration: "Length",
        size: "File size",
        played: "Recently played",
        random: "Random",
      },
      wording: {
        added: { asc: "oldest first", desc: "newest first" },
        modified: { asc: "oldest first", desc: "newest first" },
        duration: { asc: "shortest first", desc: "longest first" },
        size: { asc: "smallest first", desc: "largest first" },
        played: { asc: "least recently played", desc: "most recently played" },
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
      watchedRatio: "Watched portion",
      watched: "Watched",
      noImage: "No image",
      preparing: "Preparing",
    },
  },
  library: {
    title: "Library",
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
      visibility: "Visibility",
      makePublic: "Make public",
      makePrivate: "Make private",
      madePublic: (count: number) => `Made ${videos(count)} public`,
      madePrivate: (count: number) => `Made ${videos(count)} private`,
      visibilityFailed: (reason: string) => `Couldn't change the visibility: ${reason}`,
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
      description: "Scan to see them here.",
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
      autoHint:
        "Automatic: groups folders below a media folder that have no subfolders and 2 or more videos",
      toTag: "Turn the group into a tag",
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
      submit: "Sign in",
      throttledFor: (wait: number) =>
        `Too many sign-in attempts. Try again in ${seconds(wait)}.`,
      failed: "Couldn't sign in. Try again.",
    },
    setup: {
      title: "Create an account",
      description:
        "Create the one account for this server. To change it later, use the server's command line.",
      submit: "Create account",
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
      starting: "Starting the scan…",
      notRun: "No scan has run yet",
      scanningUnknown: "Scanning…",
      scanningCount: (completed: number, total: number) =>
        `Scanning ${formatNumber(completed)} / ${formatNumber(total)}`,
      checkingPreparation: "Checking what's left to prepare for the scanned videos",
      preparingCount: (remaining: number) =>
        `Preparing the scanned videos (${items(remaining)} left)`,
      partialFailed: (failed: number) =>
        selectPlural(failed, {
          one: `Finished with ${formatNumber(failed)} failure`,
          other: `Finished with ${formatNumber(failed)} failures`,
        }),
      failed: (reason: string) => `The scan failed: ${reason}`,
      lastScanned: (completed: number) => `The last scan processed ${items(completed)}`,
      noChanges: "The last scan found no changes",
      // 上部の進捗表示
      announce: {
        starting: "Starting the scan",
        unknownTotal: "Scanning. Counting the items",
        running: (total: number) => `Scanning ${items(total)}`,
        preparing: "Preparing the scanned videos",
        done: "The scan is complete",
        partialFailed: "The scan finished with some failures",
        failed: "The scan failed",
      },
      label: {
        starting: "Starting",
        scanning: "Scanning",
        scanningPercent: (percent: number) => `Scanning ${formatNumber(percent)}%`,
        preparing: "Preparing",
        preparingLeft: (remaining: number) =>
          `Preparing, ${formatNumber(remaining)} left`,
        partialFailed: "Some failed",
        failed: "The scan failed",
        done: "Done",
      },
      checkingRemaining: "Checking what's left",
      remaining: (remaining: number) => `${items(remaining)} left`,
      counts: (completed: number, total: number | null, failed: number) =>
        total === null
          ? `${formatNumber(completed)} / counting (${formatNumber(failed)} failed)`
          : `${formatNumber(completed)} / ${formatNumber(total)} (${formatNumber(failed)} failed)`,
      failedSeeSettings: "The scan failed. See Settings for the reason.",
      openStatus: (label: string) => `${label}. Open the scan status`,
      dismissFailure: "Dismiss the scan failure notice",
      progress: "Scan progress",
      progressPreparing: "Preparing the scanned videos",
      progressChecking: "Checking what to scan",
      breakdown: {
        label: "Preparation left",
        probe: "Analysis",
        thumbnail: "Thumbnails",
        seekThumbnail: "Seek",
        preview: "Previews",
      },
    },
  },
  settings: {
    title: "Settings",
    scanStatus: {
      heading: "Scan status",
      state: {
        notRun: "Not run",
        starting: "Starting",
        running: "Running",
        preparing: "Preparing",
        done: "Done",
        partialFailed: "Some failed",
        failed: "Failed",
        fetchFailed: "Rechecking",
      },
      nothingFound: "There was nothing to scan",
      processed: "Processed",
      total: "Total",
      failed: "Failed",
      counting: "Counting…",
      preparationLeft: "Preparation left",
      startedAt: "Started",
      finishedAt: "Finished",
      notFinished: "Not finished",
      rechecking: "Rechecking the latest status",
    },
    mediaFolders: {
      heading: "Media folders",
      description:
        "Folders on the server where vv looks for videos. After a change, run a scan with “Refresh library” at the top. Scans don't start automatically.",
      lockedWhileScanning: "You can't change media folders while a scan is running",
      list: "Added media folders",
      loading: "Loading the media folders",
      loadFailed: (reason: string) => `Couldn't load the media folders: ${reason}`,
      empty: "No media folders yet",
      emptyHint: "Add a folder to start scanning manually.",
      removing: "Removing…",
      changing: "Changing…",
      change: "Change folder",
      remove: "Remove folder",
      add: "Add folder",
      addBlocked: "You can add a folder once the current folders have loaded",
      changedElsewhere:
        "This folder was changed or removed elsewhere. Check it and try again.",
      added: "Added. Run a scan to apply the change.",
      changed: "Changed. Run a scan to apply the change.",
      removed: "Removed. A scan doesn't start automatically.",
    },
    removeDialog: {
      title: "Remove this folder?",
      warning:
        "Videos that are only in this folder leave the list. Videos that are also in another media folder stay. Playback positions and watched status are kept. A scan doesn't start automatically.",
      submit: "Remove",
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
    },
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
