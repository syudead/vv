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
type FolderWatchProblem = NonNullable<components["schemas"]["FolderWatch"]["problem"]>;
type ScanIssueKind = components["schemas"]["ScanIssueKind"];
type VideoEncoderChoice = components["schemas"]["VideoEncoderChoice"];
type EncoderUnavailableReason = components["schemas"]["EncoderUnavailableReason"];
type ComfyUIProvider = components["schemas"]["ComfyUIProvider"];
type ComfyUIModel = components["schemas"]["ComfyUIModel"];
type WorkInfoPluginState = components["schemas"]["WorkInfoPluginState"];
type WorkInfoPluginProblem = components["schemas"]["WorkInfoPluginProblem"];
type WorkInfoOperation = components["schemas"]["WorkInfoOperation"];
type SkippedTagReason = components["schemas"]["SkippedTag"]["reason"];
type WorkInfoField = components["schemas"]["WorkInfoField"];

/** ErrorDetails は API エラーが文言へ渡す値である（contracts/error-api.md §1）。 */
export interface ErrorDetails {
  limit?: number;
  tagName?: string;
  /** 作品情報プラグインの要求を拒んだか届かなかったホスト（specs/049-work-info-plugins/ui-design.md）。 */
  host?: string;
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

function groupsCount(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} group`,
    other: `${formatNumber(count)} groups`,
  });
}

function tagCount(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} tag`,
    other: `${formatNumber(count)} tags`,
  });
}

/**
 * reorderTexts は並べ替えのキーボードの案内と dnd-kit の読み上げ（持ち上げ・移動・置く・
 * 取り消し）である。位置は 1 始まり。プレイリストとグループのページが同じ文を使う。
 */
const reorderTexts = {
  reorderInstructions:
    "Press Space to pick up, the arrow keys to move, Space to drop and Esc to cancel.",
  pickedUp: (title: string, position: number, count: number) =>
    `Picked up "${title}", position ${formatNumber(position)} of ${formatNumber(count)}.`,
  movedTo: (title: string, position: number, count: number) =>
    `"${title}" moved to position ${formatNumber(position)} of ${formatNumber(count)}.`,
  dropped: (title: string, position: number, count: number) =>
    `Dropped "${title}" at position ${formatNumber(position)} of ${formatNumber(count)}.`,
  cancelled: (title: string) => `Cancelled. "${title}" is back in its place.`,
};

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

function playlistCount(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} playlist`,
    other: `${formatNumber(count)} playlists`,
  });
}

function videos(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} video`,
    other: `${formatNumber(count)} videos`,
  });
}

/**
 * creditKindTexts は人物・団体の管理画面の、種類で言い分ける文言である
 * （specs/047-video-work-info/ui-design.md「People and organizations screens」）。
 * `one`・`many` は数える名詞、`title` は見出しの名詞、`target` は統合先の欄の名札である。
 */
function creditKindTexts(one: string, many: string, title: string, target: string) {
  const count = (n: number) =>
    selectPlural(n, {
      one: `${formatNumber(n)} ${one}`,
      other: `${formatNumber(n)} ${many}`,
    });
  return {
    title,
    count,
    filteredCount: (shown: number, total: number) =>
      `${formatNumber(shown)} of ${count(total)}`,
    newButton: `New ${one}`,
    search: `Search ${many}`,
    mergeInto: `Merge into another ${one}…`,
    mergeTarget: target,
    noCandidates: `No matching ${many}`,
    searchFailed: (reason: string) => `Couldn't search ${many}: ${reason}`,
    gone: `This ${one} no longer exists, so the list was reloaded`,
    emptyTitle: `No ${many} yet`,
    noMatches: (query: string) => `No ${many} match "${query}"`,
    loadFailed: (reason: string) => `Couldn't load ${many}: ${reason}`,
    staleList: (reason: string) =>
      `Couldn't load ${many}: ${reason}. The list below may not match the current search and sort.`,
    loadingMore: `Loading more ${many}…`,
    listChanged: `${title} were added or removed elsewhere, so the rest of this list may be out of date.`,
  };
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
  provider_failed: "ComfyUI couldn't start the edit.",
  plugin_too_large: "The file is over 16 MB.",
  plugin_changed: "The plugin was replaced. Review it again.",
  plugin_unusable: "The plugin can't be used.",
  plugin_not_approved: "The plugin is no longer approved.",
  plugin_settings_missing: "The plugin needs settings first.",
  plugin_failed: "The plugin failed.",
  plugin_timed_out: "The plugin didn't finish in 30 seconds and was stopped.",
  plugin_busy: "Another plugin is running. Try again in a moment.",
  operation_unsupported: "The plugin can't do that.",
  work_info_fetch_stale: "The result is out of date. Fetch it again.",
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
  no_scan: "No scan has run yet.",
  directory_not_found: "That folder wasn't found.",
  file_unavailable: "The video file can't be opened.",
  media_folders_changed:
    "The media folders were changed elsewhere. Reload and try again.",
  group_target_not_found: "A video or group in the request no longer exists.",
  group_cycle: "A group can't be put inside itself or inside a group it contains.",
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
  auto_tagging_off:
    "Auto-tagging is turned off. Turn it on under Settings, Auto-tagging.",
  comfyui_url_invalid: "Enter an http or https URL",
  comfyui_api_key_missing: "Enter the Comfy Cloud API key",
  thumbnail_edit_not_found:
    "This edit has ended, because it was opened again elsewhere or left idle. Close it and start again.",
  thumbnail_edit_not_ready: "ComfyUI isn't set up",
  thumbnail_edit_mask_invalid: "The painting doesn't match the frame",
  thumbnail_edit_reference_invalid: "The reference image can't be read",
  thumbnail_edit_no_result: "There is no generated image yet",
  thumbnail_image_invalid: "The image can't be read.",
  thumbnail_upload_too_large: "The image is too large: up to 64 MB and 120 megapixels.",
  thumbnail_upload_invalid:
    "The file isn't a PNG, JPEG or still WebP, or it can't be read.",
  content_changed: "The video changed in the meantime.",
  unreachable: "ComfyUI can't be reached",
  unauthorized: "Comfy Cloud rejected the API key",
  insufficient_credits: "Comfy Cloud has no credits left",
  missing_models:
    "ComfyUI is missing files the edit needs; test the connection in Settings to see which",
  workflow_rejected: "ComfyUI rejected the workflow",
  run_failed: "The edit failed in ComfyUI",
  no_output: "ComfyUI returned no image",
  timed_out: "No result after 10 minutes",
  // 作品情報と人物・団体の理由は項目名を入れない。画面が `field` の示す入力の下に出す
  // （specs/047-video-work-info/ui-design.md「Error reasons」）。
  work_info_control_characters: "Contains characters that can't be used.",
  work_info_too_long: ({ limit }) =>
    limit === undefined ? "Too long." : `Too long: up to ${characters(limit)}.`,
  work_info_too_many: ({ limit }) =>
    limit === undefined ? "Too many." : `Too many: up to ${formatNumber(limit)}.`,
  work_info_invalid_date: "Enter a date as YYYY, YYYY-MM or YYYY-MM-DD.",
  work_info_invalid_url: "Enter a full web address starting with http:// or https://.",
  work_info_invalid_language: "Enter a language tag such as ja or en-US.",
  work_info_duplicate: "Already listed.",
  work_info_empty: "This can't be empty.",
  undo_conflict: "It changed in the meantime, so it can't be undone.",
  undo_expired: "This can no longer be undone.",
  person_not_found: "This person no longer exists.",
  organization_not_found: "This organization no longer exists.",
  organization_parent_cycle:
    "The parent can't be the organization itself or one inside it.",
  merge_same_person: "A person can't be merged into themselves.",
  merge_same_organization: "An organization can't be merged into itself.",
  external_id_taken: "Another entry already has this ID.",
  // 作品情報プラグインの実行の失敗（specs/049-work-info-plugins/ui-design.md の Error text）。
  blocked_address: ({ host }) =>
    host === undefined
      ? "The plugin tried to reach VVMDM itself or a device on your network."
      : `The plugin tried to reach ${host}, which is VVMDM itself or a device on your network.`,
  network_error: ({ host }) =>
    host === undefined
      ? "The plugin couldn't reach a site."
      : `The plugin couldn't reach ${host}.`,
  memory_exceeded: "The plugin used more than 128 MB of memory and was stopped.",
  output_too_large: "The plugin's answer was over 1 MB.",
  // タグの種類（specs/051-tag-kinds/ui-design.md）。名前の重複は tagName に種類の名前が入る。
  tag_kind_not_found: "This kind no longer exists.",
  tag_kind_name_taken: ({ tagName }) =>
    tagName === undefined
      ? "A kind with that name already exists."
      : `A kind named "${tagName}" already exists.`,
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
    breadcrumbs: "Breadcrumbs",
    more: "More",
    loading: "Loading",
    sidebar: "Sidebar",
    toggleSidebar: "Toggle sidebar",
    notifications: "Notifications",
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
    component: {
      variant: "Variant",
      states: {
        normal: "Normal",
        hover: "Hover",
        focus: "Keyboard focus",
        pressed: "Pressed",
        selected: "Selected",
        disabled: "Disabled",
      },
      none: "None",
      action: "Save",
      iconAction: "Add",
      text: "Kyoto Arashiyama",
      placeholder: "Type a title",
      label: "Title",
      description: "Shown under the thumbnail.",
      error: "Enter a title.",
      form: "Field with label, description and error",
      option: (n: number) => `Option ${String(n)}`,
      choose: "Choose an option",
      search: "Search options",
      empty: "No matching option",
      filter: "Travel",
      setting: "Public",
      zoom: "Card size",
      grid: "Grid",
      list: "List",
      view: "View",
    },
    overlay: {
      title: "Overlays and feedback",
      vvTitle: "vv components",
      states: {
        normal: "Normal",
        hover: "Hover",
        focus: "Keyboard focus",
        pressed: "Pressed",
        selected: "Selected",
        disabled: "Disabled",
        open: "Open",
      },
      openDialog: "Open dialog",
      openAlertDialog: "Delete tag…",
      dialogTitle: "Merge tags",
      dialogDescription: "Videos tagged with the source tag move to the target tag.",
      dialogBody: "Kyoto Arashiyama, morning walk",
      merge: "Merge",
      alertTitle: "Delete the tag “travel”?",
      alertDescription: "It is removed from 12 videos. This can't be undone.",
      delete: "Delete",
      popoverTrigger: "View and sort",
      popoverTitle: "Card size",
      popoverDescription: "Larger cards show fewer per row.",
      menuTrigger: "Visibility",
      menuLabel: "Selected videos",
      makePublic: "Make public",
      makePrivate: "Make private",
      favorite: "Favorite",
      removeTag: "Remove tag",
      tooltip: "Refresh the library",
      tabs: { all: "All", tentative: "Tentative", rejected: "Rejected" },
      toast: "Show a toast",
      toastMessage: "Added 3 videos to favorites",
      badges: {
        tag: "travel",
        count: "12",
        selected: "kyoto",
        warning: "Stalled",
        error: "Failed",
        done: "Done",
      },
      alerts: {
        errorTitle: "Couldn't load the videos",
        errorDescription: "Check the connection and try again.",
        warningTitle: "Playback is stalling",
        warningDescription: "The file is still being converted.",
        infoTitle: "Autoplay is off",
        infoDescription: "Press play to start the video.",
      },
      empty: {
        title: "No videos yet",
        description: "Add a media folder in settings to start a scan.",
      },
      loading: "Loading more videos",
      progress: "Scan progress",
      breadcrumb: { root: "Media", parent: "Travel", current: "Kyoto 2024" },
      kbd: "Search syntax",
      sidebar: {
        library: "Library",
        folders: "Folders",
        tags: "Tags",
        settings: "Settings",
      },
      thumbnail: { duration: "49:10", watched: "Share of the video watched" },
      favoriteOff: "Add to favorites",
      favoriteOn: "Remove from favorites",
      tentativeTag: "kyoto",
    },
    // 画面の型の見本（web/src/designSystem/blocks）。中身は見本のデータで、型を写した
    // 画面は自分の文言に差し替える（web/registry/rules/patterns.md）。
    pattern: {
      intro:
        "Each page skeleton with its sections and states, filled with sample data. Copy the block of the skeleton a new screen needs.",
      items: "Items",
      names: {
        listPage: "List page",
        adminTablePage: "Admin table page",
        settingsPage: "Settings page",
        detailPage: "Detail page",
        centeredForm: "Centered form",
        dialogs: "Form dialog and confirm dialog",
        editorDialog: "Editor dialog",
        formPage: "Form page",
        groupedList: "Grouped list",
        listState: (state: string) => `List page: ${state}`,
      },
      states: {
        loading: "Loading",
        empty: "Empty",
        error: "Error",
        loadingMore: "Loading more",
        loadMoreFailed: "Load more failed",
      },
      library: "Library",
      videos,
      addFolder: "Add folder",
      search: "Search videos",
      filters: "Filters",
      viewAndSort: "View and sort",
      view: "View",
      grid: "Grid",
      list: "List",
      cardSize: "Card size",
      sort: "Sort",
      sortAdded: "Date added",
      sortTitle: "Title",
      sortDuration: "Duration",
      selected: (n: number) => `${formatNumber(n)} selected`,
      clearSelection: "Clear selection",
      addTag: "Add tag",
      hide: "Hide",
      select: (title: string) => `Select ${title}`,
      videoTitles: {
        arashiyama: "Kyoto Arashiyama, morning walk",
        harbour: "Harbour lights at dusk",
        ridge: "Snow on the northern ridge",
        market: "Night market in Taipei",
        train: "Coastal train, window seat",
        eikando: "Autumn leaves at Eikando",
        shibuya: "Rainy street in Shibuya",
        lighthouse: "Lighthouse on the cape",
      },
      videoMeta: (duration: string, added: string) => `${duration} · ${added}`,
      addedAgo: (days: number) =>
        selectPlural(days, {
          one: `${formatNumber(days)} day ago`,
          other: `${formatNumber(days)} days ago`,
        }),
      loading: "Loading videos",
      loadingMore: "Loading more videos",
      emptyTitle: "No videos yet",
      emptyDescription: "Add a folder to start scanning for videos.",
      errorTitle: "Couldn't load videos",
      errorDescription: "The server didn't respond. Check the connection and try again.",
      loadMoreFailedTitle: "Couldn't load more videos",
      loadMoreFailedDescription: "The videos above are kept.",
      retry: "Retry",
      tags: "Tags",
      tagCount: (n: number) =>
        selectPlural(n, {
          one: `${formatNumber(n)} tag`,
          other: `${formatNumber(n)} tags`,
        }),
      newTag: "New tag",
      allTags: "All",
      tentativeTags: "Tentative",
      searchTags: "Search tags",
      tagTable: "Tags",
      name: "Name",
      kind: "Kind",
      videoCount: "Videos",
      tentative: "Tentative",
      confirmed: "Confirmed",
      selectAll: "Select all tags",
      selectTag: (name: string) => `Select ${name}`,
      tagActions: (name: string) => `Actions for ${name}`,
      rename: "Rename",
      merge: "Merge",
      delete: "Delete",
      tagNames: {
        travel: "Travel",
        kyoto: "Kyoto",
        night: "Night",
        trains: "Trains",
        autumn: "Autumn",
        coast: "Coast",
      },
      settings: "Settings",
      settingsDescription: "Settings apply to this server and every viewer.",
      librarySection: "Library",
      librarySectionDescription: "How the library lists and plays videos.",
      showHidden: "Show hidden videos",
      showHiddenDescription: "List videos hidden from guests as well.",
      defaultSort: "Default sort",
      defaultSortDescription: "The order a new viewer sees first.",
      autoplay: "Play the next video",
      autoplayDescription: "Start the next video in the list when one ends.",
      serverSection: "Server",
      serverSectionDescription: "Read only; set when the server starts.",
      version: "Version",
      dataFolder: "Data folder",
      port: "Port",
      rescan: "Scan folders now",
      rescanDescription: "Look for added and removed videos in every folder.",
      scan: "Scan",
      back: "Back",
      play: "Play",
      videoTitle: "Kyoto Arashiyama, morning walk",
      videoSubtitle: "Added 2 days ago",
      description: "Description",
      descriptionText:
        "A slow walk from the bamboo grove to the river in the first light, with the sound of the forest and the morning trains.",
      information: "Information",
      related: "Related videos",
      duration: "Duration",
      resolution: "Resolution",
      fileSize: "File size",
      added: "Added",
      signIn: "Sign in",
      signInDescription: "Sign in to manage this library.",
      userName: "User name",
      password: "Password",
      signInFooter: "Ask the owner of this server for an account.",
      createTag: "Create",
      newTagDescription: "A tag groups videos across folders.",
      tagName: "Name",
      tagNameDescription: "Shown on cards and in filters.",
      tagNotes: "Notes",
      cancel: "Cancel",
      deleteTag: "Delete tag",
      deleteTagTitle: (name: string) => `Delete "${name}"?`,
      deleteTagDescription:
        "The tag is removed from 12 videos. The videos themselves are kept.",
      history: "History",
      historyEntries: "Watch history",
      today: "Today",
      yesterday: "Yesterday",
      removeEntry: "Remove from history",
      editImage: "Edit image",
      editImageDetail: "Frame at 1:02",
      editImageStage: "Image",
      editImageNotes: "Instructions",
      editImageNotesHint: "Optional: what to change in the image.",
      editImageApply: "Apply",
      close: "Close",
      formPageTitle: "Edit details",
      formPageAside: "Video",
      formPageFile: "harbour-at-dusk.mp4",
      formPageFacts: "49:10 · 2.4 GB · 1920 × 1080",
      formPageBasics: "Basics",
      formPageMore: "More",
      formPageName: "Name",
      formPageNotes: "Description",
      formPageNotesHint: "Shown under the video's details.",
      formPageCode: "Code",
      save: "Save",
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
    items,
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
      // プレイリストに足す操作の読み上げ名（specs/044-playlists/ui-design.md「Words」）。
      addToPlaylist: (title: string) => `Add "${title}" to a playlist`,
      // グループに足す操作の読み上げ名（specs/048-groups/ui-design.md「Words」）。
      addToGroup: (title: string) => `Add "${title}" to a group`,
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
      // 人物・団体のチップも並ぶ（specs/047-video-work-info/ui-design.md「Words」の Library）。
      label: "Filtering by",
      list: "Tags in the filter",
      remove: "Remove the tag filter",
      removeNamed: (name: string) => `Remove the filter for ${name}`,
      removePerson: "Remove the person filter",
      removeOrganization: "Remove the organization filter",
      personGone: "Removed a person who no longer exists from the filter",
      organizationGone: "Removed an organization that no longer exists from the filter",
      // タグの種類のチップ（specs/051-tag-kinds/ui-design.md「Words」の Library chip）。
      kindChip: (name: string) => `Kind: ${name}`,
      removeKind: "Remove the kind filter",
      removeKindNamed: (name: string) => `Remove the filter for the kind "${name}"`,
      kindGone: "Removed a kind that no longer exists from the filter",
    },
    // 絞り込みのポップオーバーのタグの種類の行（specs/051-tag-kinds/ui-design.md「Words」）。
    tagKindFilter: {
      label: "Tag kind",
      any: "Any kind",
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
      addToPlaylist: (name: string) => `Add group "${name}" to a playlist`,
      addToGroup: (name: string) => `Add group "${name}" to a group`,
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
      addToPlaylist: "Add to playlist",
      newGroup: "New group",
      newGroupNeedsMore: "Select at least one video, or a group and something else",
      addToGroup: "Add to group",
      visibility: "Visibility",
      makePublic: "Make public",
      makePrivate: "Make private",
      madePublic: (count: number) => `Made ${videos(count)} public`,
      madePrivate: (count: number) => `Made ${videos(count)} private`,
      visibilityFailed: (reason: string) => `Couldn't change the visibility: ${reason}`,
      bundle: "Bundle as versions",
      bundleOverLimit: (limit: number) => `Bundle up to ${videos(limit)} at a time`,
      // グループのページだけに出る（specs/048-groups/ui-design.md「Selection bar」）。
      removeFromGroup: "Remove from group",
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
      toTag: "Turn the group into a tag…",
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
    // タグの種類の行の名札・絞り込み・チップ（specs/051-tag-kinds/ui-design.md「Words」）。
    // 種類の名前は利用者のデータなので訳さずに埋め込む。
    kindFilter: {
      label: "Kind",
      any: "Any kind",
      none: "No kind",
      chip: (name: string) => `Kind: ${name}`,
      removeUnnamed: "Remove the kind filter",
      gone: "Removed a kind that no longer exists from the filter",
    },
    kindLabel: {
      prefix: "Kind:",
    },
    noKindTags: (name: string) => `No tags of the kind "${name}"`,
    noKindlessTags: "No tags without a kind",
    noFilterMatches: "No tags match the current search and filters",
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
    // 「Kinds」のタブ（specs/051-tag-kinds/ui-design.md「Words」「Kinds tab」）。
    kinds: {
      tab: "Kinds",
      table: "Kinds",
      description:
        "A tag has one kind or none. Set it from the Tags tab's selection bar. Deleting a kind leaves its tags without one.",
      newKind: "New kind",
      loading: "Loading kinds…",
      columns: {
        name: "Name",
        tags: "Tags",
      },
      tagCount,
      open: (name: string) => `Show the tags of the kind "${name}"`,
      create: {
        label: "New kind name",
        placeholder: "Kind name",
        submit: "Create",
        submitting: "Creating…",
      },
      renameLabel: (name: string) => `New name for "${name}"`,
      deleteDialog: {
        title: (name: string) => `Delete the kind "${name}"?`,
        used: (count: number) =>
          selectPlural(count, {
            one: `The ${formatNumber(count)} tag of this kind stays, without a kind. This can't be undone.`,
            other: `The ${formatNumber(count)} tags of this kind stay, without a kind. This can't be undone.`,
          }),
        unused: "No tags have this kind. This can't be undone.",
        submit: "Delete",
        submitting: "Deleting…",
      },
      deleted: (name: string) => `Deleted the kind "${name}"`,
      gone: "This kind no longer exists, so the list was reloaded",
      empty: {
        title: "No kinds yet",
        description:
          "A kind groups tags, such as Genre, Work or Place. Add one to give tags a kind.",
      },
      loadFailed: "Couldn't load the kinds",
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
      // 選択バーの「Set kind…」（specs/051-tag-kinds/ui-design.md「Set kind」）。
      setKind: "Set kind…",
      noKinds: "No kinds yet. Add them on the Kinds tab.",
      kindSet: (name: string, applied: number, skipped: number) =>
        skipped === 0
          ? `Set the kind "${name}" on ${tagCount(applied)}`
          : `Set the kind "${name}" on ${tagCount(applied)}. ${formatNumber(skipped)} already had it.`,
      kindCleared: (applied: number, skipped: number) =>
        skipped === 0
          ? `Removed the kind from ${tagCount(applied)}`
          : `Removed the kind from ${tagCount(applied)}. ${formatNumber(skipped)} had none.`,
      kindGone: "That kind no longer exists, so the kinds were reloaded",
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
  // 人物と団体の管理画面（specs/047-video-work-info/ui-design.md「Words」の「People and
  // organizations screens」）。並び順の文言はタグの画面（tags.sort）と同じものを使う。
  credits: {
    person: creditKindTexts("person", "people", "People", "Person to merge into"),
    organization: creditKindTexts(
      "organization",
      "organizations",
      "Organizations",
      "Organization to merge into",
    ),
    loading: "Loading…",
    columns: {
      name: "Name",
      videos: "Videos",
    },
    empty: {
      description: "Add them from a video's details, or create one here.",
    },
    clearSearch: "Clear search",
    create: {
      label: "Name",
      placeholder: "Name",
    },
    row: {
      open: (name: string) => `Open the library filtered by ${name}`,
      aliases: (list: string) => `Aliases: ${list}`,
      aliasWithLanguage: (alias: string, language: string) => `${alias} (${language})`,
      partOf: (parent: string) => `Part of ${parent}`,
      // 名前の下の 1 行の、親と別名の区切り。
      secondLine: (parts: readonly string[]) => parts.join(" · "),
      videoCount: videos,
      renameLabel: (name: string) => `New name for "${name}"`,
      rename: "Rename",
      details: "Edit details",
      more: "More actions",
      delete: "Delete…",
      actions: "Actions",
    },
    details: {
      title: (name: string) => `Edit "${name}"`,
      aliases: "Aliases",
      externalIds: "External IDs",
      parent: "Parent organization",
      alias: "Alias",
      language: "Language",
      languagePlaceholder: "ja",
      languageDescription: "Optional. A language tag such as ja or en-US.",
      removeAlias: "Remove alias",
      addAlias: "Add alias",
      source: "Source",
      value: "Value",
      removeId: "Remove ID",
      addId: "Add ID",
      parentNone: "None",
      parentSearch: "Search organizations",
      parentEmpty: "No matching organizations",
      parentsFailed: (reason: string) => `Couldn't load organizations: ${reason}`,
      submit: "Save",
      submitting: "Saving…",
    },
    mergeDialog: {
      title: (name: string) => `Merge "${name}"`,
      aliasHint: (alias: string) => `Alias: ${alias}`,
      videoCount: videos,
      warning: (source: string, count: number, target: string) =>
        count === 0
          ? `"${source}" isn't on any videos. Its name, aliases and IDs become aliases and IDs of "${target}", and it leaves the list. This can't be undone.`
          : `The ${videos(count)} with "${source}" get "${target}" instead. "${source}"'s name, aliases and IDs become aliases and IDs of "${target}", and "${source}" leaves the list. This can't be undone.`,
      submit: "Merge",
      submitting: "Merging…",
    },
    merged: (source: string, target: string) => `Merged "${source}" into "${target}"`,
    deleteDialog: {
      title: (name: string) => `Delete "${name}"?`,
      text: (name: string, count: number) =>
        count === 0
          ? `"${name}" isn't on any videos. This can't be undone.`
          : `"${name}" will be removed from ${videos(count)}. This can't be undone.`,
      organizationNote: "Organizations inside it stay and lose their parent.",
      submit: "Delete",
      submitting: "Deleting…",
    },
    deleted: (name: string) => `Deleted "${name}"`,
  },
  folderGrouping: {
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
      /** 窓より前のメンバーを読むボタン（狭い画面。広い画面ではスクロールで読む）。 */
      earlier: (count: number) =>
        count === 1
          ? "Show 1 earlier video"
          : `Show ${formatNumber(count)} earlier videos`,
      moreFailed: "Couldn't load more of the group",
    },
    // プレイリストの文脈で開いた動画ページ（specs/044-playlists/ui-design.md「Words」）。
    // 名前と題名（利用者のデータ）はそのまま埋め込む。
    playlist: {
      line: (name: string, position: number, count: number) =>
        position > 0
          ? `Playlist "${name}", item ${formatNumber(position)} of ${formatNumber(count)}. Open the playlist`
          : `Playlist "${name}". Open the playlist`,
      previous: "Previous item",
      next: "Next item",
      end: "End of playlist",
      endAnnouncement: (name: string) =>
        `Playback finished. End of the playlist "${name}"`,
      gone: "This video is no longer in the playlist. Playback stops after it.",
      skipped: (title: string) => `Skipped "${title}": it can't be played here.`,
      skippedLast: (title: string) => `Can't play "${title}". End of the playlist.`,
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
    // ComfyUI でコマを編集してサムネイルにする入口・編集画面・生成した画像の項目
    // （specs/045-comfyui-thumbnails/ui-design.md「Words」の Video page）。
    thumbnailEdit: {
      entry: "Edit thumbnail with ComfyUI",
      setupTitle: "Set up ComfyUI first",
      setupBody: "Save a ComfyUI connection in Settings.",
      openSettings: "Open Settings",
      startFailed: (reason: string) => `Couldn't start editing the thumbnail: ${reason}`,
      title: "Edit thumbnail",
      frameAt: (time: string) => `Frame at ${time}`,
      // 編集を何から始めるかのメニューと、今のサムネイルから始めたときの編集画面の文言
      // （specs/046-thumbnail-image-upload/ui-design.md「Words」）。
      frameOnScreen: "Frame on screen",
      currentThumbnail: "Current thumbnail",
      thumbnailFrameAt: (time: string) => `Current thumbnail, frame at ${time}`,
      thumbnailSavedImage: "Current thumbnail, saved image",
      close: "Close",
      tools: "Painting tools",
      paint: "Paint",
      erase: "Erase",
      brushSize: "Brush size",
      undo: "Undo",
      clearPainting: "Clear painting",
      paintedArea: "Painted area",
      paintingHint:
        "Paint over what to change. Leave it unpainted to change the whole frame.",
      paintingHintImage:
        "Paint over what to change. Leave it unpainted to change the whole image.",
      instructions: "Instructions",
      instructionsPlaceholder: "For example: remove the caption at the bottom",
      instructionsHint: "Optional when you paint: the painted area is then cleaned up.",
      reference: "Reference image",
      addReference: "Add image…",
      removeReference: "Remove reference image",
      referenceHint:
        "Its colours and mood are used, and its objects may be copied. PNG, JPEG or WebP, up to 20 MB.",
      referenceRefused: "Choose a PNG, JPEG or WebP image up to 20 MB.",
      generate: "Generate",
      generateAgain: "Generate again",
      states: {
        queued: "Waiting in the queue",
        preparing: "Preparing",
        running: "Generating",
      },
      step: (value: number, max: number) =>
        `Step ${formatNumber(value)} of ${formatNumber(max)}`,
      progress: "Progress",
      elapsed: "Elapsed",
      longWait: "The first run can take a minute while ComfyUI loads its models.",
      cancel: "Cancel",
      cancelled: "Cancelled. The thumbnail is unchanged.",
      runFailed: (reason: string) => `Couldn't generate the image: ${reason}`,
      frame: "Frame",
      thumbnail: "Thumbnail",
      generated: "Generated",
      cropNote: "Cropped to the video's shape, as the thumbnail will be.",
      apply: "Use as thumbnail",
      applying: "Applying…",
      changeAndGenerate: "Change and generate again",
      discard: "Discard",
      showResult: "Show result",
      applyFailed: (reason: string) => `Couldn't use the image: ${reason}`,
      confirmTitle: "Discard this edit?",
      confirmDescription:
        "The run is cancelled and the generated image is lost. The thumbnail stays as it is.",
      keepEditing: "Keep editing",
      generatedThumbnail: "Generated thumbnail",
      useFrameInstead: "Use the frame instead",
    },
    // 手元の画像をサムネイルにする入口とアップロードした画像の項目
    // （specs/046-thumbnail-image-upload/ui-design.md「Words」の Video page）。
    thumbnailUpload: {
      entry: "Upload an image as thumbnail",
      refusedType: "Choose a PNG, JPEG or WebP image.",
      refusedSize: "Choose an image up to 64 MB.",
      failed: (reason: string) => `Couldn't upload the image: ${reason}`,
      uploadedThumbnail: "Uploaded thumbnail",
      preview: {
        title: "Preview thumbnail",
        close: "Cancel and close",
        fit: "How the image fits",
        contain: "Whole image",
        cover: "Fill frame",
        containHint:
          "The whole image shows. The rest of the frame is filled with a blurred copy.",
        coverHint:
          "The image fills the frame. Drag it to choose what shows, and zoom in to show less.",
        unknownAspect: "The video's size is unknown, so the image keeps its own shape.",
        zoom: "Zoom",
        stage: "Thumbnail preview",
        stageCover: "Thumbnail preview. Drag or use the arrow keys to move the image.",
        apply: "Use as thumbnail",
        cancel: "Cancel",
      },
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
    // 情報欄の作品情報の行と編集の入口（specs/047-video-work-info/ui-design.md「Words」の
    // Video page, work details row）。役割の名前は 1 人なら単数、2 人以上なら複数。
    workInfo: {
      label: "Work details",
      code: "Code",
      released: "Released",
      recorded: "Recorded",
      published: "Published",
      performer: (count: number) =>
        selectPlural(count, { one: "Performer", other: "Performers" }),
      director: (count: number) =>
        selectPlural(count, { one: "Director", other: "Directors" }),
      photographer: (count: number) =>
        selectPlural(count, { one: "Photographer", other: "Photographers" }),
      streamer: (count: number) =>
        selectPlural(count, { one: "Streamer", other: "Streamers" }),
      maker: (count: number) => selectPlural(count, { one: "Maker", other: "Makers" }),
      labelRole: (count: number) =>
        selectPlural(count, { one: "Label", other: "Labels" }),
      links: "Links",
      ids: "IDs",
      externalId: (source: string, value: string) => `${source}: ${value}`,
      showVideosWith: (name: string) => `Show videos with ${name}`,
      more: (count: number) => `+${formatNumber(count)} more`,
      showAll: (count: number, label: string) =>
        `Show all ${formatNumber(count)} ${label.toLowerCase()}`,
      showFewer: "Show fewer",
      showMore: "Show more",
      showLess: "Show less",
      edit: "Edit details",
    },
    // 作品情報プラグインからの取得（specs/049-work-info-plugins/ui-design.md「Words」の
    // Video page text）。
    workInfoFetch: {
      entry: "Fetch work info from a plugin",
      title: "Fetch work info",
      description: "Nothing changes until you apply.",
      plugin: "Plugin",
      queryBoth: "Search words or the work's page URL",
      querySearch: "Search words",
      queryFetch: "The work's page URL",
      notUrl: "Paste a page URL starting with http:// or https://.",
      search: "Search",
      fetch: "Fetch",
      searching: "Searching…",
      fetching: "Fetching…",
      matches: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} match`,
          other: `${formatNumber(count)} matches`,
        }),
      matchList: "Matches",
      openPage: "Open page",
      noMatches: "No matches.",
      noMatchesFetch: "No matches. Try other words or paste the work's page URL.",
      source: (plugin: string) => `From ${plugin} · `,
      back: "Back to the matches",
      now: "Now",
      fetched: "Fetched",
      // 比較の行のラベル（specs/052-plugin-work-info-fields/ui-design.md「Words」）。役割は複数形。
      fields: {
        title: "Title",
        tags: "Tags",
        code: "Code",
        description: "Description",
        "dates.release": "Released",
        "dates.recorded": "Recorded",
        "dates.published": "Published",
        "people.performer": "Performers",
        "people.director": "Directors",
        "people.photographer": "Photographers",
        "people.streamer": "Streamers",
        "organizations.maker": "Makers",
        "organizations.label": "Labels",
        urls: "Links",
        externalIds: "IDs",
      } satisfies Record<WorkInfoField, string>,
      notInResult: "Not in the result",
      nothingToAdd: "Nothing to add",
      // チップの中の従の印。読み上げは「{name}, New」になる。
      newMark: "New",
      newHint: "Created when you apply",
      notLinked: "Not linked",
      notLinkedHint: {
        person: "Matches more than one person, so it isn't linked or created",
        organization: "Matches more than one organization, so it isn't linked or created",
      },
      fetchedAs: (name: string) => `Fetched as "${name}"`,
      onVideoFetchedAs: (name: string) => `On the video. Fetched as "${name}"`,
      sameAsNow: "Same as now",
      onVideo: "On the video",
      tentative: "Tentative",
      newCount: (count: number) => `${formatNumber(count)} new`,
      // タグの行の数。m は動画にまだ無い、選べるタグの数（specs/056-plugin-tooling/ui-design.md）。
      tagsSelected: (selected: number, selectable: number) =>
        `${formatNumber(selected)} of ${formatNumber(selectable)} selected`,
      more: (count: number) => `+${formatNumber(count)} more`,
      showFewer: "Show fewer",
      apply: "Apply selected",
      applying: "Applying…",
      applied: "Applied to the video.",
      resultTitle: "Applied, but some values were left out",
      // 1 行は「{label}: {value} — {理由}」で、値だけを太くする。
      skippedLabel: (label: string) => `${label}: `,
      skipped: {
        rejected: "you rejected this tag before",
        invalid: "not a valid tag name",
      } satisfies Record<SkippedTagReason, string>,
      skippedValue: {
        invalidDate: "not a date in the form YYYY-MM-DD, YYYY-MM or YYYY",
        invalidUrl: "not a full URL starting with http:// or https://",
        invalidOther: "too long or contains characters that can't be used",
        tooMany: (items: string) => `over the limit for ${items}`,
        ambiguous: {
          person: "matches more than one person",
          organization: "matches more than one organization",
        },
      },
      // 「over the limit for …」に入る、件数に上限のある行の名前。
      limitItems: {
        "people.performer": "performers",
        "people.director": "directors",
        "people.photographer": "photographers",
        "people.streamer": "streamers",
        "organizations.maker": "makers",
        "organizations.label": "labels",
        urls: "links",
        externalIds: "IDs",
      },
      done: "Done",
      stale: "This result is out of date. Fetch it again to apply it.",
      fetchAgain: "Fetch again",
      settingsMissing: (plugin: string, labels: readonly string[]) =>
        `${plugin} needs ${formatList(labels)} first.`,
      openSettings: "Open Settings",
      searchFailed: (reason: string) => `Couldn't search: ${reason}`,
      fetchFailed: (reason: string) => `Couldn't fetch: ${reason}`,
      applyFailed: (reason: string) => `Couldn't apply: ${reason}`,
      pluginSaid: (message: string) => `The plugin said: "${message}"`,
    },
    // 全画面の編集画面（specs/047-video-work-info/ui-design.md「Words」の Editor）。
    detailsEditor: {
      title: "Edit details",
      documentTitle: (title: string) => `Edit details · ${title}`,
      video: "Video",
      back: "Back to the video",
      basics: "Basics",
      credits: "Credits",
      people: "People",
      organizations: "Organizations",
      name: "Name",
      code: "Code",
      description: "Description",
      released: "Released",
      recorded: "Recorded",
      published: "Published",
      performers: "Performers",
      directors: "Directors",
      photographers: "Photographers",
      streamers: "Streamers",
      makers: "Makers",
      labels: "Labels",
      links: "Links",
      ids: "IDs",
      datePlaceholder: "YYYY-MM-DD",
      dateHint: "A year or a year and month is enough.",
      addPerformer: "Add a performer",
      addDirector: "Add a director",
      addPhotographer: "Add a photographer",
      addStreamer: "Add a streamer",
      addMaker: "Add a maker",
      addLabel: "Add a label",
      create: (name: string) => `Create "${name}"`,
      alias: (alias: string) => `Alias: ${alias}`,
      videoCount: videos,
      remove: (name: string) => `Remove ${name}`,
      newMark: "New",
      newChip: (name: string) => `${name}, new`,
      link: "Link",
      removeLink: "Remove link",
      addLink: "Add link",
      source: "Source",
      value: "Value",
      removeId: "Remove ID",
      addId: "Add ID",
      cancel: "Cancel",
      save: "Save",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't save the details: ${reason}`,
      createdNotSaved: (count: number, reason: string) =>
        selectPlural(count, {
          one: `1 person was created, but the details weren't saved: ${reason}`,
          other: `${formatNumber(count)} people were created, but the details weren't saved: ${reason}`,
        }),
      searchPeopleFailed: (reason: string) => `Couldn't search people: ${reason}`,
      searchOrganizationsFailed: (reason: string) =>
        `Couldn't search organizations: ${reason}`,
      discardTitle: "Discard changes?",
      discardText: (title: string) => `The changes to "${title}" haven't been saved.`,
      keepEditing: "Keep editing",
      discard: "Discard",
      loadFailed: (reason: string) => `Couldn't load the video: ${reason}`,
      gone: "This video no longer exists.",
      retry: "Retry",
    },
    visibility: {
      label: "Show to people who aren't signed in",
      public: "Public",
      private: "Private",
      failed: (reason: string) => `Couldn't change the visibility: ${reason}`,
    },
    // Group line と、その後のグループ操作（specs/048-groups/ui-design.md「Video page」）。
    group: {
      line: (name: string, position: number, count: number) =>
        `Group "${name}", video ${formatNumber(position)} of ${formatNumber(count)}. Open the group`,
      actions: "Group actions",
      remove: "Remove from this group",
      toTag: "Turn the group into a tag…",
      dissolve: "Dissolve…",
      removed: (name: string) => `Removed from "${name}"`,
      removeFailed: (reason: string) => `Couldn't remove from the group: ${reason}`,
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
      confirm: (name: string) => `Keep ${name} (suggested by auto-tagging)`,
      reject: (name: string) => `Reject ${name} for this video`,
      settleFailed: (reason: string) => `Couldn't update the suggested tag: ${reason}`,
    },
    autoTag: {
      run: "Suggest tags",
      queued:
        "Asking the classifier for tags. Matching tags are added when it answers, which can take a few minutes.",
      alreadyRunning: "This video is already being tagged.",
      failed: (reason: string) => `Couldn't start auto-tagging: ${reason}`,
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
  history: {
    title: "History",
    documentTitle: "History",
    list: "Watch history",
    day: {
      today: "Today",
      yesterday: "Yesterday",
      // 行の読み上げ名に入れる日: 見出しの 1 行目と 2 行目（「Today, Oct 10」）。
      full: (name: string, date: string) => `${name}, ${date}`,
    },
    // 行の再生位置と長さ（「16:05 / 42:18」）。どちらも formatDuration で整えた文字列。
    position: (position: string, duration: string) => `${position} / ${duration}`,
    resume: "Resume",
    resumeFor: (title: string) => `Resume ${title}`,
    startOver: "Start over",
    startOverFor: (title: string) => `Start ${title} over`,
    // 行のリンクと × の読み上げ名。日だけを入れ、時刻は入れない（日より細かく刻まない）。
    entryLink: (title: string, duration: string, day: string) =>
      duration === ""
        ? `${title}, played ${day}`
        : `${title}, ${duration}, played ${day}`,
    remove: "Remove from history",
    removeFor: (title: string, day: string) =>
      `Remove "${title}" played ${day} from history`,
    notInLibrary: "Not in the library",
    unknownTitle: "Unknown video",
    clear: "Clear history…",
    clearDialog: {
      title: "Clear watch history?",
      description:
        "Every entry is removed. Playback positions and watched marks stay as they are.",
      submit: "Clear",
      submitting: "Clearing…",
      failed: (reason: string) => `Couldn't clear the history: ${reason}`,
    },
    empty: {
      title: "No watch history",
      description: "Videos you play are listed here, newest first.",
    },
    loadFailed: "Couldn't load the history",
    search: {
      label: "Search titles",
    },
    jump: {
      title: "Jump to date",
      none: "No dates to jump to",
      loadFailed: "Couldn't load the dates",
    },
    noMatch: {
      title: "No history matches these conditions",
    },
  },
  // プレイリスト（specs/044-playlists/ui-design.md「Words」）。
  playlists: {
    title: "Playlists",
    documentTitle: "Playlists",
    // 一覧のカードのリンクの読み上げ名（「Morning, 3 items」）。
    rowLabel: (name: string, count: number) => `${name}, ${items(count)}`,
    items,
    count: playlistCount,
    new: "New playlist",
    newDialog: {
      title: "New playlist",
      submit: "Create",
      submitting: "Creating…",
      failed: (reason: string) => `Couldn't create the playlist: ${reason}`,
    },
    nameLabel: "Name",
    nameRule: "Use a name of 1 to 100 characters.",
    empty: {
      title: "No playlists yet",
      description:
        "Create a playlist, then add videos and groups to it from the library.",
    },
    loadFailed: "Couldn't load the playlists",
    // プレイリストの画面（/playlists/{id}）。
    detail: {
      // 見出しの件数（「12 items · 1:32:10」）。長さの分からない項目があれば長さを省く。
      count: (count: number, duration: string) =>
        duration === "" ? items(count) : `${items(count)} · ${duration}`,
      play: "Play",
      playFor: (name: string) => `Play "${name}" from the top`,
      rename: "Rename…",
      delete: "Delete…",
      table: "Items",
      empty: {
        title: "No items yet",
        description:
          "Add videos and groups with Add to playlist on a card, on the video page, or in the selection bar.",
        action: "Open library",
      },
      gone: {
        title: "This playlist no longer exists",
        description: "It was deleted, maybe in another tab.",
        action: "Back to playlists",
      },
      loadFailed: "Couldn't load the playlist",
    },
    item: {
      playFrom: (title: string) => `Play from ${title}`,
      reorder: (title: string) => `Reorder "${title}"`,
      ...reorderTexts,
      remove: "Remove from playlist",
      removeFor: (title: string) => `Remove "${title}" from the playlist`,
      moveFailed: (reason: string) => `Couldn't move the item: ${reason}`,
      removeFailed: (reason: string) => `Couldn't remove the item: ${reason}`,
    },
    renameDialog: {
      title: "Rename playlist",
      submit: "Save",
      submitting: "Saving…",
      failed: (reason: string) => `Couldn't rename the playlist: ${reason}`,
    },
    deleteDialog: {
      title: (name: string) => `Delete "${name}"?`,
      description: "The playlist is deleted. Its videos and groups stay in the library.",
      submit: "Delete",
      submitting: "Deleting…",
      failed: (reason: string) => `Couldn't delete the playlist: ${reason}`,
    },
    deleted: (name: string) => `Deleted "${name}"`,
    // 動画とグループをプレイリストに足すピッカー（specs/044-playlists/ui-design.md
    // 「Add to playlist」「Words」）。名前（利用者のデータ）はそのまま埋め込む。
    add: {
      label: "Add to playlist",
      search: "Search or create a playlist",
      list: "Playlists",
      create: (name: string) => `Create "${name}"`,
      none: "No playlists yet. Type a name to create one.",
      nameTooLong: "Use a name of up to 100 characters",
      loading: "Loading…",
      loadFailed: "Couldn't load the playlists",
      added: (count: number, name: string) => `Added ${items(count)} to "${name}"`,
      existing: (count: number, name: string) =>
        count === 1 ? `Already in "${name}"` : `${items(count)} already in "${name}"`,
      partly: (added: number, existing: number, name: string) =>
        `Added ${items(added)} to "${name}"; ${formatNumber(existing)} already there`,
      nothing: "Nothing was added",
      failed: (reason: string) => `Couldn't add to the playlist: ${reason}`,
    },
  },
  // グループのページ（/groups/{id}）と、グループをタグに変える窓（specs/048-groups/ui-design.md
  // 「Words」）。名前とパス（利用者のデータ）はそのまま埋め込む。
  groups: {
    // 道筋の帯（Breadcrumb）の名前。
    trail: "Group path",
    nameLabel: "Name",
    // プレイリストの名前と同じ規則（research.md R-14）。
    nameRule: "Use a name of 1 to 100 characters.",
    created: (name: string) => `Created "${name}"`,
    // 選択バーの「New group」の窓。説明は何がグループになるかを言う。グループ 1 つだけの選択では
    // 開かないので、「1 group becomes one group」は出ない。
    newDialog: {
      title: "New group",
      description: (videoCount: number, groupCount: number) => {
        const parts: string[] = [];
        if (videoCount > 0) parts.push(videos(videoCount));
        if (groupCount > 0) parts.push(groupsCount(groupCount));
        const verb = videoCount + groupCount === 1 ? "becomes" : "become";
        return `${parts.join(" and ")} ${verb} one group`;
      },
      submit: "Create",
      submitting: "Creating…",
      failed: (reason: string) => `Couldn't create the group: ${reason}`,
    },
    // 「Add to group」のピッカー（カード・行・動画ページ・選択バーで共有）。
    add: {
      label: "Add to group",
      search: "Search or create a group",
      list: "Groups",
      create: (name: string) => `Create "${name}"`,
      none: "No groups yet. Type a name to create one.",
      nameTooLong: "Use a name of up to 100 characters",
      loading: "Loading…",
      loadFailed: "Couldn't load the groups",
      // 既にあるものはサーバーが数えずに飛ばすので、件数は言わない。
      added: (name: string) => `Added to "${name}"`,
      failed: (reason: string) => `Couldn't add to the group: ${reason}`,
    },
    detail: {
      // 見出しの件数（「12 videos · 1:32:10」）。長さはサーバーの合計を formatDuration で
      // 整えた文字列で、分からなければ空にして件数だけにする。
      count: (count: number, duration: string) =>
        duration === "" ? videos(count) : `${videos(count)} · ${duration}`,
      // 自動グループのフォルダの行（「From the folder {path}」）。後ろのパスはフォルダの
      // ページへのリンクとして画面が続ける。
      fromFolder: "From the folder",
      play: "Play",
      playFor: (name: string) => `Play "${name}"`,
      table: "Items",
      gone: {
        title: "This group no longer exists",
        description: "It was dissolved, maybe in another tab.",
        action: "Back to library",
      },
      loadFailed: "Couldn't load the group",
      rename: "Rename…",
      toTag: "Turn the group into a tag…",
      dissolve: "Dissolve…",
    },
    item: {
      reorder: (title: string) => `Reorder "${title}"`,
      ...reorderTexts,
      remove: "Remove from group",
      removeFor: (title: string) => `Remove "${title}" from the group`,
      moveFailed: (reason: string) => `Couldn't move the item: ${reason}`,
      removeFailed: (reason: string) => `Couldn't remove from the group: ${reason}`,
    },
    removed: (count: number, name: string) => `Removed ${items(count)} from "${name}"`,
    reloaded: "The group changed in the meantime, so it was reloaded",
    renameDialog: {
      title: "Rename group",
      submit: "Save",
      submitting: "Saving…",
      failed: (reason: string) => `Couldn't rename the group: ${reason}`,
    },
    dissolveDialog: {
      title: (name: string) => `Dissolve "${name}"?`,
      description: "The group is dissolved. Its videos and groups stay in the library.",
      folder:
        "Its folder is no longer grouped automatically. Choose Automatic on the folder page to group it again.",
      submit: "Dissolve",
      submitting: "Dissolving…",
      failed: (reason: string) => `Couldn't dissolve the group: ${reason}`,
    },
    dissolved: (name: string) => `Dissolved "${name}"`,
    // グループをタグに変える確認の窓と、その成功のトースト（ui-design.md「Turning into a tag」
    // 「Words」）。グループページ、動画ページ、フォルダのメニューが共有する。
    tagDialog: {
      title: (name: string) => `Turn "${name}" into a tag?`,
      description: (name: string) =>
        `The tag "${name}" is added to every video in the group, and the group is dissolved.`,
      submit: "Turn into a tag",
      submitting: "Turning into a tag…",
      failed: (reason: string) => `Couldn't turn the group into a tag: ${reason}`,
    },
    tagged: (name: string, created: boolean) =>
      created
        ? `Created the tag "${name}" and dissolved the group`
        : `Added the tag "${name}" and dissolved the group`,
  },
  // ドラッグで並べ替え・グループ化・マージ・バージョン化する操作と、その取り消しの通知
  // （specs/050-drag-organize/ui-design.md「Words」）。
  organize: {
    drop: {
      putIn: "Put in",
      putInOrMerge: "Put in or merge…",
      refused: "Can't drop here",
      groupOrBundle: "Group or bundle…",
    },
    // 落とした先に開くメニュー。名前は落とした先のカードの題名か名前。
    menu: {
      label: (name: string) => `Dropped on "${name}"`,
      makeGroup: "Make a group",
      nest: "Put in as a child group",
      merge: "Merge",
    },
    // 「Make a group」の後の、新しいグループの名前の欄。hint は仮の名前を埋め込む。
    name: {
      label: "Group name",
      hint: (name: string) => `Enter keeps the name, Esc keeps "${name}"`,
    },
    done: {
      reordered: (name: string) => `Reordered "${name}"`,
      grouped: (name: string) => `Made the group "${name}"`,
      moved: (count: number, name: string) => `Moved ${items(count)} to "${name}"`,
      movedOut: (count: number, name: string) => `Moved ${items(count)} out of "${name}"`,
      // mixed は動画もいっしょにマージしたとき（数は項目で数える）。
      merged: (count: number, name: string, mixed: boolean) =>
        `Merged ${mixed ? items(count) : groupsCount(count)} into "${name}"`,
    },
    failed: {
      reorder: (reason: string) => `Couldn't reorder: ${reason}`,
      group: (reason: string) => `Couldn't make the group: ${reason}`,
      move: (reason: string) => `Couldn't move: ${reason}`,
      merge: (reason: string) => `Couldn't merge: ${reason}`,
    },
    // ドラッグの層の読み上げの案内（dnd-kit の screenReaderInstructions、描かない）。カードは
    // キーボードでは持ち上がらず、キーボードでは 048 のメニューを使う（research.md R-7）。
    instructions:
      "Press and move a card to pick it up, then drop it on another card or a group to organize. With the keyboard, use the card's buttons instead.",
    undo: "Undo",
    undone: "Undone",
    undoFailed: {
      conflict: "Couldn't undo: it changed in the meantime",
      expired: "This can no longer be undone",
      other: (reason: string) => `Couldn't undo: ${reason}`,
    },
    // dnd-kit の読み上げ。位置は 1 始まり。
    announce: {
      pickedUp: (title: string, count: number) =>
        count === 1 ? `Picked up "${title}"` : `Picked up ${items(count)}`,
      over: (name: string, label: string) => `Over "${name}": ${label}`,
      between: (position: number, count: number) =>
        `Between cards, position ${formatNumber(position)} of ${formatNumber(count)}`,
      dropped: (name: string) => `Dropped on "${name}"`,
      droppedAt: (position: number, count: number) =>
        `Dropped at position ${formatNumber(position)} of ${formatNumber(count)}`,
      cancelled: "Cancelled, nothing changed",
    },
  },
  tagName: {
    required: "Enter a name",
    controlCharacters: "Line breaks and tabs aren't allowed",
    tooLong: (limit: number, length: number) =>
      `Use ${characters(limit)} or fewer (currently ${formatNumber(length)})`,
  },
  // 人物・団体の名前を打つ CreditCommand の理由（specs/047-video-work-info/ui-design.md「Words」）。
  creditName: {
    required: "Enter a name",
    controlCharacters: "Line breaks and tabs aren't allowed",
    tooLong: (limit: number) => `Up to ${characters(limit)}`,
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
      people: "People",
      organizations: "Organizations",
      history: "History",
      playlists: "Playlists",
      duplicates: "Duplicates",
      settings: "Settings",
      login: "Sign in",
      logout: "Sign out",
      loggingOut: "Signing out…",
      logoutFailed: "Couldn't sign out",
      menu: "Menu",
      closeMenu: "Close menu",
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
      // 自動の取り込み（origin watch）の言葉。走っているあいだは何も出さないので、
      // 終わったあとの状態だけがある（specs/042-folder-watch-import/ui-design.md「Words」）。
      watchStatus: {
        done: "Auto-imported",
        partial: "Auto-import: some failed",
        failed: "Auto-import failed",
      },
      watchAnnounce: {
        partial: (failed: number) =>
          `Auto-import finished with some failures. ${videos(failed)} may not be usable.`,
        failed: "Auto-import failed.",
      },
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
      dismiss: "Hide the scan progress",
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
      // 取り込みを始める button と、押せない理由（issue 830）。
      start: "Scan library",
      starting: "Scanning…",
      runningReason: "A scan is running. You can start another when it finishes.",
      needsMediaFolder: "Add a media folder below to scan.",
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
    autoImport: {
      heading: "Auto-import",
      description:
        "Files added, removed, moved or renamed in a media folder are picked up as they happen, without a scan. Changes made while it's off or while VVMDM isn't running are picked up by “Scan library” under Scan status.",
      label: "Pick up changes as they happen",
      state: {
        off: "Off. Changes are picked up by the next scan.",
        noFolder: "Add a media folder below to watch it.",
        starting: "Starting to watch the media folders…",
        active: "Watching the media folders.",
        limited: "Watching, but some changes may be missed.",
      },
      loading: "Loading the auto-import settings",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't change the setting: ${reason}`,
      loadFailed: (reason: string) => `Couldn't load the auto-import settings: ${reason}`,
      // 問題の文。{path} の前と後に分け、道筋（利用者のデータ）は画面が <code> で挟む。
      problem: {
        watch_limit: {
          title: "Some folders aren't watched",
          before: "The limit on watched folders was reached, so changes under ",
          after:
            " aren't picked up. Raise the limit (fs.inotify.max_user_watches on Linux) and turn auto-import off and on, then start a scan to pick up what was missed.",
        },
        events_lost: {
          title: "Some changes were lost",
          before:
            "Too many changes arrived at once and some were lost. Start a scan under Scan status to pick them up.",
          after: "",
        },
        folder_unreachable: {
          title: "A media folder can't be reached",
          before: "",
          after:
            " can't be reached. Changes there aren't picked up until it is back and auto-import is turned off and on. Then start a scan to pick up what was missed.",
        },
        permission_denied: {
          title: "A folder can't be read",
          before: "VVMDM can't read ",
          after:
            ". Changes there aren't picked up until it can, and auto-import is turned off and on. Then start a scan to pick up what was missed.",
        },
      } satisfies Record<
        FolderWatchProblem,
        { title: string; before: string; after: string }
      >,
      /** 問題が道筋を持たないときに、{path} の代わりに入れる言葉。 */
      unknownPath: "a media folder",
    },
    ollama: {
      heading: "Ollama",
      description:
        "Where VVMDM reaches Ollama and which model it uses. Auto-tagging and duplicate judging use it; the test works whether they are on or off.",
      endpoint: "Ollama URL",
      endpointHint: "Where Ollama listens, for example http://127.0.0.1:11434.",
      model: "Model",
      modelHint: "clef-flash is fast; clef is larger and more accurate.",
      save: "Save",
      saving: "Saving…",
      saved: "Ollama settings saved",
      check: "Save and test connection",
      checking: "Testing…",
      checkOk: "Ollama is running and has the model.",
      checkFailed: (reason: string) => `The connection test failed: ${reason}`,
      saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      loading: "Loading the Ollama settings",
      loadFailed: (reason: string) => `Couldn't load the Ollama settings: ${reason}`,
    },
    autoTagging: {
      heading: "Auto-tagging",
      description:
        "Asks the classifier set under Ollama which existing tags fit each video, from its thumbnail, frames from across the video, title, file name and folder. No new tags are created.",
      enabled: "Use auto-tagging",
      enabledHint:
        "Off: no video is sent to the classifier and the video page has no Suggest tags button. Suggested tags already added can still be kept or rejected.",
      afterImport: "Tag new videos automatically",
      afterImportHint: "After a video is imported, ask the classifier for its tags.",
      threshold: "Probability to add",
      thresholdHint:
        "A tag is added when the classifier's probability is at least this value (above 0, up to 1).",
      tentativeThreshold: "Probability to suggest",
      tentativeThresholdHint:
        "Between this value and the one above, the tag is added as a suggestion to keep or reject on the video page. The same value as above turns suggestions off.",
      save: "Save",
      saving: "Saving…",
      saved: "Auto-tagging settings saved",
      saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      check: "Test judging",
      checking: "Asking the classifier… Loading the model can take a few minutes.",
      checkOk: "The classifier answered.",
      checkFailed: (reason: string) => `The classifier didn't answer: ${reason}`,
      loading: "Loading the auto-tagging settings",
      loadFailed: (reason: string) =>
        `Couldn't load the auto-tagging settings: ${reason}`,
      queue: (queued: number, running: number, done: number, failed: number) =>
        `Waiting ${formatNumber(queued)} · Tagging ${formatNumber(running)} · Done ${formatNumber(done)} · Failed ${formatNumber(failed)}`,
      lastError: "Last failure",
      runMissing: "Tag videos not yet tagged",
      runAll: "Tag all videos again",
      started: (count: number) =>
        count === 0
          ? "There were no videos to tag"
          : selectPlural(count, {
              one: `${formatNumber(count)} video was queued for auto-tagging`,
              other: `${formatNumber(count)} videos were queued for auto-tagging`,
            }),
      startFailed: (reason: string) => `Couldn't start auto-tagging: ${reason}`,
    },
    versionJudging: {
      heading: "Duplicate judging",
      description:
        "Judges each pair the scan lists as a possible duplicate: the two files are compared frame by frame, and the classifier set under Ollama is asked only about the frames that differ. Same videos are bundled as versions, different videos leave Duplicates, and the rest wait there for you.",
      label: "Judge possible duplicates automatically",
      hint: "Off: every possible duplicate waits on Duplicates for you. On: pairs that match throughout are bundled even without Ollama; pairs with a differing part wait until an Ollama connection is saved.",
      loading: "Loading the duplicate judging settings",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      loadFailed: (reason: string) =>
        `Couldn't load the duplicate judging settings: ${reason}`,
    },
    comfyui: {
      heading: "ComfyUI",
      description:
        "Where VVMDM sends a video frame to edit it into a thumbnail. VVMDM brings the workflow; a ComfyUI you run yourself needs the model files listed after a test.",
      guide: "How to set up ComfyUI",
      opensInNewTab: "(opens in a new tab)",
      provider: "Runs on",
      providers: {
        self_hosted: {
          label: "My own ComfyUI",
          hint: "ComfyUI you run on this PC or another one.",
        },
        cloud: {
          label: "Comfy Cloud",
          hint: "Comfy's hosted service. Uses your API key and its credits.",
        },
      } satisfies Record<ComfyUIProvider, { label: string; hint: string }>,
      model: "Model",
      models: {
        flux2_klein_4b: {
          label: "FLUX.2 [klein] 4B",
          hint: "Fast, and the best at removing captions. About 12 GB of model files.",
        },
        qwen_image_edit_2511: {
          label: "Qwen-Image-Edit-2511",
          hint: "Takes the colours and mood of a reference image well. About 31 GB of model files and a 24 GB GPU.",
        },
      } satisfies Record<ComfyUIModel, { label: string; hint: string }>,
      url: "ComfyUI URL",
      urlHint: "Where ComfyUI listens, for example http://127.0.0.1:8188.",
      apiKey: "API key",
      apiKeyHint: "Create one in your Comfy Cloud account.",
      apiKeySavedHint: "A key is saved. Type a new one to replace it.",
      keyStillSaved: "A Comfy Cloud key is still saved.",
      forgetKey: "Forget the key",
      forgetting: "Forgetting…",
      keyForgotten: "Comfy Cloud key forgotten",
      save: "Save",
      saving: "Saving…",
      saved: "ComfyUI settings saved",
      check: "Save and test connection",
      checking: "Testing…",
      checkOk: {
        self_hosted: "Connected to ComfyUI.",
        cloud: "Connected to Comfy Cloud.",
      } satisfies Record<ComfyUIProvider, string>,
      checkFailed: (reason: string) => `The connection test failed: ${reason}`,
      saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      missingHeading: "ComfyUI is missing what the thumbnail edit needs:",
      putIn: (folder: string) => `put it in ${folder}`,
      download: "Download",
      updateForNode: "Update ComfyUI to get this node",
      loading: "Loading the ComfyUI settings",
      loadFailed: (reason: string) => `Couldn't load the ComfyUI settings: ${reason}`,
    },
    workInfoPlugins: {
      heading: "Work-info plugins",
      description:
        "Plugins fetch work information from other sites for the video page. Approve a plugin before it can run.",
      guide: "How plugins work",
      opensInNewTab: "(opens in a new tab)",
      list: "Added work-info plugins",
      loading: "Loading the work-info plugins",
      loadFailed: (reason: string) => `Couldn't load the work-info plugins: ${reason}`,
      add: "Add plugin",
      adding: "Adding…",
      fileInput: "Plugin file",
      tooLarge: "Choose a file up to 16 MB.",
      addFailed: (reason: string) => `Couldn't add the plugin: ${reason}`,
      added: (name: string) => `Added ${name}. Approve it to use it.`,
      replaced: (name: string) => `Replaced ${name}. Approve it again to use it.`,
      addedUnusable: (name: string) => `Added ${name}, but it can't be used.`,
      empty: "No plugins yet",
      emptyHint: "Add a .wasm plugin file to fetch titles and tags on the video page.",
      states: {
        approved: "Approved",
        unapproved: "Not approved",
        invalid: "Can't be used",
      } satisfies Record<WorkInfoPluginState, string>,
      version: (version: string) => `v${version}`,
      operations: {
        search: "Search",
        fetch: "Fetch by URL",
      } satisfies Record<WorkInfoOperation, string>,
      network: "Reaches internet sites",
      settingSet: (label: string) => `${label} set`,
      settingNotSet: (label: string) => `${label} not set`,
      settingsSummary: (items: readonly string[]) => `Settings: ${items.join(", ")}`,
      requiredMissing: (labels: readonly string[]) =>
        `Needs a value: ${labels.join(", ")}`,
      approve: "Review and approve",
      settings: "Settings",
      more: (name: string) => `More actions for ${name}`,
      revoke: "Revoke approval",
      delete: "Delete",
      revoking: "Revoking…",
      deleting: "Deleting…",
      rowFailed: (name: string, reason: string) => `Couldn't change ${name}: ${reason}`,
      problems: {
        not_wasm: "The file isn't a WebAssembly module.",
        manifest_missing: "The file has no plugin information for VVMDM.",
        manifest_invalid: "The plugin information in the file is invalid.",
        contract_unsupported: "The plugin is made for another version of VVMDM.",
        import_unsupported: "The plugin asks for functions VVMDM doesn't offer.",
        export_invalid: "The file isn't a plugin VVMDM can run.",
        table_too_large: "The plugin asks for more memory tables than VVMDM allows.",
      } satisfies Record<WorkInfoPluginProblem, string>,
      approval: {
        title: (name: string) => `Approve ${name}?`,
        description:
          "Approved plugins can run when you fetch work information on a video page.",
        version: "Version",
        file: "File",
        can: "Can",
        pluginDescription: "Description",
        digest: "SHA-256",
        networkTitle: "This plugin can reach any site on the internet",
        networkBody:
          "It can send what it fetches and the values of its settings to those sites. VVMDM blocks its requests to VVMDM itself and to devices on your network.",
        settingsHeading: "Settings it receives",
        secret: "Secret",
        required: "Required",
        noSettings: "It receives no settings.",
        submit: "Approve",
        changed:
          "The plugin was replaced while this was open. Close this and review it again.",
        failed: (reason: string) => `Couldn't approve the plugin: ${reason}`,
      },
      settingsDialog: {
        title: (name: string) => `Settings for ${name}`,
        description:
          "Values are sent only to this plugin when it runs. Changing them keeps the approval.",
        required: "Required",
        secretSaved: "Saved. Type a new value to replace it.",
        secretNotSaved: "Not set.",
        forget: "Forget",
        forgotten: "Removed when you save.",
        undo: "Undo",
        submit: "Save",
        saved: (name: string) => `Saved the settings for ${name}.`,
        saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      },
      deleteDialog: {
        title: (name: string) => `Delete ${name}?`,
        description:
          "The plugin and the values of its settings are removed. Videos keep what was already applied.",
        submit: "Delete",
        deleting: "Deleting…",
        deleted: (name: string) => `Deleted ${name}.`,
        failed: (reason: string) => `Couldn't delete the plugin: ${reason}`,
      },
    },
    mediaFolders: {
      heading: "Media folders",
      description:
        "Folders on the server where VVMDM looks for videos. After adding or changing a folder, start a scan with “Scan library” under Scan status. Auto-import doesn't start one.",
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
