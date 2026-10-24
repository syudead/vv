import type { components } from "../api/gen/openapi";
import { formatDecimal, formatList, formatNumber, selectPlural } from "./intl";

// Central UI catalog; components access it through messages.ts. See docs/design-docs/i18n.md.
//
// Use formatNumber and selectPlural for interpolated counts.
// Insert user names and paths verbatim.
// Generated Record types make missing API error translations fail type checking.

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
type WorkInfoPluginKind = components["schemas"]["WorkInfoPluginKind"];
type WorkInfoPluginProblem = components["schemas"]["WorkInfoPluginProblem"];
type WorkInfoOperation = components["schemas"]["WorkInfoOperation"];
type SkippedTagReason = components["schemas"]["SkippedTag"]["reason"];
type PersonPluginItem = components["schemas"]["PersonPluginItem"];
type WorkInfoField = components["schemas"]["WorkInfoField"];
type ActivityKind = components["schemas"]["ActivityKind"];
type ActivityFeature = components["schemas"]["ActivityFeature"];
type ActivityDecider = components["schemas"]["ActivityDecider"];
type ActivityStage = components["schemas"]["ActivityStage"];
type MediaInfoHdr = components["schemas"]["MediaInfoVideo"]["hdr"];

export interface ErrorDetails {
  limit?: number;
  tagName?: string;
  host?: string;
}

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

/** "Dissolve 2 groups and delete 1 smart folder", leaving out a zero part. */
function collectionRemoval(
  groupCount: number,
  folderCount: number,
  dissolve: string,
  remove: string,
): string {
  const parts = [
    ...(groupCount === 0 ? [] : [`${dissolve} ${groupsCount(groupCount)}`]),
    ...(folderCount === 0 ? [] : [`${remove} ${smartFolders(folderCount)}`]),
  ];
  const text = parts.join(" and ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function tagCount(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} tag`,
    other: `${formatNumber(count)} tags`,
  });
}

/** Drag positions are one-based; share keyboard guidance across playlists and groups. */
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

function selectedTags(count: number): string {
  return selectPlural(count, {
    one: "The selected tag",
    other: `The ${formatNumber(count)} selected tags`,
  });
}

function someOf(applied: number, selected: number): string {
  return `${formatNumber(applied)} of the ${formatNumber(selected)} selected tags ${applied === 1 ? "is" : "are"}`;
}

function pronoun(count: number): string {
  return count === 1 ? "It" : "They";
}

/** count counts subject tags; videoCount zero uses the no-videos wording. */
function removal(count: number, videoCount: number): string {
  if (videoCount === 0)
    return count === 1 ? "isn't on any videos" : "aren't on any videos";
  return `will be removed from ${videos(videoCount)}`;
}

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

function smartFolders(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} smart folder`,
    other: `${formatNumber(count)} smart folders`,
  });
}

function videos(count: number): string {
  return selectPlural(count, {
    one: `${formatNumber(count)} video`,
    other: `${formatNumber(count)} videos`,
  });
}

/** one/many are count nouns, title is a heading noun, and target labels the merge destination. */
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
  plugin_kind_mismatch: "This plugin is for the other kind of information.",
  operation_unsupported: "The plugin can't do that.",
  work_info_fetch_stale: "The result is out of date. Fetch it again.",
  person_external_id_missing:
    "This person has no ID the plugin accepts. Search by name instead.",
  person_link_ambiguous:
    "This person has several links the plugin handles. Choose one first.",
  person_link_unsupported: "The plugin doesn't handle this link.",
  activity_not_retryable: "This can no longer be retried",
  activity_not_undoable: "This can no longer be undone",
  invalid_smart_rule: "A condition is out of range.",
  smart_folder_not_found: "That smart folder no longer exists.",
  smart_folder_cycle:
    "A smart folder can't be put inside itself or inside one of its children.",
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
  version_judging_off:
    "Duplicate judging is turned off. Turn it on under Settings, Duplicate judging.",
  version_not_rejudgeable: "This pair is already waiting for the AI or being judged.",
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
  // Omit field names because the UI places these errors beneath the field.
  //
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
  work_info_out_of_range: "This value is outside the allowed range.",
  undo_conflict: "It changed in the meantime, so it can't be undone.",
  undo_expired: "This can no longer be undone.",
  person_not_found: "This person no longer exists.",
  organization_not_found: "This organization no longer exists.",
  organization_parent_cycle:
    "The parent can't be the organization itself or one inside it.",
  merge_same_person: "A person can't be merged into themselves.",
  person_image_not_found: "This person has no photo.",
  person_image_invalid:
    "The file isn't a PNG, JPEG or still WebP up to 120 megapixels, or it can't be read.",
  person_image_too_large: "The image is too large: up to 64 MB.",
  merge_same_organization: "An organization can't be merged into itself.",
  external_id_taken: "Another entry already has this ID.",
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
  tag_kind_not_found: "This kind no longer exists.",
  tag_kind_name_taken: ({ tagName }) =>
    tagName === undefined
      ? "A kind with that name already exists."
      : `A kind named "${tagName}" already exists.`,
  face_candidate_not_found: "These two people are no longer offered as a match.",
  face_appearance_not_found: "This face is no longer linked to this person.",
  face_recognition_off:
    "Face recognition is turned off. Turn it on under Settings, Face recognition.",
  face_recognition_unsupported: "vv has no face runtime for this platform.",
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
    selectAll: "Select all",
    clearAll: "Clear",
    sameCount: (count: number) => `Same as now (${formatNumber(count)})`,
    hideSame: "Hide values that stay",
    fillsEmpty: "Fills empty fields",
    differsFromNow: "Differs from now",
    replacesNote: "Checked rows replace the current value.",
    addsToLists: "Adds to lists",
    breadcrumbs: "Breadcrumbs",
    more: "More",
    loading: "Loading",
    sidebar: "Sidebar",
    toggleSidebar: "Toggle sidebar",
    notifications: "Notifications",
    imageUpload: {
      zone: "Drop an image here, or click to choose a file",
      zoneHint: "You can also paste an image while this is open.",
    },
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
        activity: "Activity",
        count: "3",
        countLabel: "3 need attention",
      },
      filter: {
        status: "Status",
        period: "Period",
        all: "All",
        attention: "Needs attention",
        running: "Running",
        done: "Done",
        anyTime: "Any time",
        today: "Today",
      },
      thumbnail: { duration: "49:10", watched: "Share of the video watched" },
      favoriteOff: "Add to favorites",
      favoriteOn: "Remove from favorites",
      tentativeTag: "kyoto",
    },
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
        profilePage: "Profile page",
        faceGrid: "Face grid",
        groupedList: "Grouped list",
        stepsPage: "Steps page",
        cardList: "Card list",
        recordPage: "Record page",
        listState: (state: string) => `List page: ${state}`,
      },
      recordTitle:
        "Kyoto Arashiyama, morning walk through the bamboo grove and along the Hozu river, recorded on a long holiday weekend.mp4",
      recordMessage:
        "ffmpeg exited with status 1\n[mov,mp4] moov atom not found; the file may be truncated or still being copied into the media folder",
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
      settingsPages: {
        library: "Library",
        playback: "Playback",
        server: "Server",
      },
      videosSection: "Videos",
      videosSectionDescription: "How the library lists videos and finds new ones.",
      playerSection: "Player",
      playerSectionDescription: "What the player does when a video ends.",
      showHidden: "Show hidden videos",
      showHiddenDescription: "List videos hidden from guests as well.",
      defaultSort: "Default sort",
      defaultSortDescription: "The order a new viewer sees first.",
      autoplay: "Play the next video",
      autoplayDescription: "Start the next video in the list when one ends.",
      detailsSection: "Details",
      detailsSectionDescription: "Read only; set when the server starts.",
      serverNotice: "Restart the server to change these values.",
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
      profilePageLabel: "Person",
      profilePageName: "Hana Mori",
      profilePageAliases: "Also known as 森はな",
      profilePageEdit: "Edit",
      profilePageBorn: "Born",
      profilePageBornValue: "1990 (age 34–35)",
      profilePageHeight: "Height",
      profilePageHeightValue: "160 cm",
      profilePageAge: "Age",
      profilePageAgeValue: "34–35",
      profilePagePerformer: "Performer",
      profilePageCount: "4 videos",
      faceGridLabel: "People",
      faceGridLoading: "Loading people",
      faceGridPeople: [
        { name: "Hana Mori", meta: "34 · 12 videos" },
        { name: "Ren Aoki", meta: "3 videos" },
        { name: "Mio Sato", meta: "28–29 · 5 videos" },
        { name: "Kaito Fujiwara Takahashi", meta: "1 video" },
      ],
      save: "Save",
      home: "Home",
      steps: "Setup steps",
      stepNames: { folders: "Folders", topics: "Topics", finish: "Finish" },
      stepOf: (current: number, total: number) =>
        `Step ${formatNumber(current)} of ${formatNumber(total)}`,
      stepDone: (name: string) => `${name}, done`,
      stepCurrent: (name: string) => `${name}, current step`,
      stepsTitle: "What do you watch?",
      stepsDescription: "Pick a few topics. You can change them later.",
      topics: "Topics",
      skip: "Skip",
      continue: "Continue",
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
    suggestions: {
      label: "Search suggestions",
      searchFor: (text: string) => `Search for “${text}”`,
      people: "People",
      tags: "Tags",
      organizations: "Organizations",
      kind: (name: string) => `Kind: ${name}`,
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
        inProgress: "In progress",
        watched: "Watched",
      },
      favoritesOnly: "Favorites only",
      clear: "Clear filters",
      apply: "Apply",
    },
    sort: {
      heading: "Sort by",
      savedOrder: "Saved order",
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
      //
      favorite: (title: string) => `Favorite "${title}"`,
      favoriteFailed: (reason: string) => `Couldn't change the favorite: ${reason}`,
      addToPlaylist: (title: string) => `Add "${title}" to a playlist`,
      addToGroup: (title: string) => `Add "${title}" to a group`,
      morePerformers: (count: number) => `+${formatNumber(count)}`,
      watchedRatio: "Watched portion",
      // Position and duration are already formatted with formatDuration.
      //
      scrubTime: (position: string, duration: string) => `${position} / ${duration}`,
      watched: "Watched",
      noImage: "No image",
      preparing: "Preparing",
    },
  },
  library: {
    title: "Library",
    resultsLabel: "Search results",
    // Library total counts cards, including one per group, not videos.
    resultCount: items,
    empty: {
      title: "No videos yet",
      description: "Put videos in a media folder and scan to see them here.",
      setUp: "Set up library",
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
    activeTags: {
      label: "Filtering by",
      list: "Tags in the filter",
      remove: "Remove the tag filter",
      removeNamed: (name: string) => `Remove the filter for ${name}`,
      removePerson: "Remove the person filter",
      removeOrganization: "Remove the organization filter",
      personGone: "Removed a person who no longer exists from the filter",
      organizationGone: "Removed an organization that no longer exists from the filter",
      kindChip: (name: string) => `Kind: ${name}`,
      removeKind: "Remove the kind filter",
      removeKindNamed: (name: string) => `Remove the filter for the kind "${name}"`,
      kindGone: "Removed a kind that no longer exists from the filter",
    },
    tagFilter: {
      kindHint: "Tag kind",
      kindOption: (name: string) => `Tag kind ${name}`,
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
    collection: {
      more: "More",
      remaining: (count: number) => `+${formatNumber(count)}`,
      playPreview: (name: string, title?: string) =>
        title ? `Play "${title}" from "${name}"` : `Play video from "${name}"`,
    },
    smartFolder: {
      label: (name: string, count: number, watched: number) =>
        watched >= 1
          ? `${name}, smart folder of ${videos(count)}, ${formatNumber(watched)} watched`
          : `${name}, smart folder of ${videos(count)}`,
      labelLoading: (name: string) => `${name}, smart folder`,
      favorite: (name: string) => `Favorite smart folder "${name}"`,
      addToPlaylist: (name: string) => `Add smart folder "${name}" to a playlist`,
      select: (name: string) => `Select the smart folder "${name}"`,
    },
    bulkEdit: {
      title: "Edit videos",
      detail: (count: number) => `${videos(count)}`,
      counting: "Counting…",
      loading: "Loading the values of the selected videos…",
      tagsAndPeople: "Tags and people",
      organizationsDatesVisibility: "Organizations, dates and visibility",
      filter: (field: string) => `Filter ${field.toLowerCase()}`,
      showAll: (count: number) => `Show all ${formatNumber(count)}`,
      showFewer: "Show fewer",
      summaryFailed: (reason: string) => `Couldn't read the selected videos: ${reason}`,
      allExcluded: "Every video is left out. Press one on the left to edit it.",
      targets: "Videos to edit",
      stageHint: "Press a video to leave it out of the edit.",
      includeAll: (count: number) =>
        selectPlural(count, {
          one: "Put back 1 video",
          other: `Put back ${formatNumber(count)} videos`,
        }),
      tileLabel: (label: string, included: boolean) =>
        included ? `${label}, edited` : `${label}, left out`,
      groupMark: (count: number) => videos(count),
      smartFolderMark: "Smart folder",
      excluded: "Left out",
      howTo:
        "Each value shows how many selected videos have it. Press it to add it to every video, press again to remove it from every video, and once more to leave it as it is.",
      tags: "Tags",
      addTag: "Add a tag",
      chipKeep: "unchanged",
      chipAdd: "will be added to every video",
      chipRemove: "will be removed from every video",
      chipLabel: (label: string, count: number, total: number, state: string) =>
        `${label}, on ${formatNumber(count)} of ${videos(total)}, ${state}`,
      ratio: (count: number, total: number) =>
        `${formatNumber(count)}/${formatNumber(total)}`,
      toAll: (total: number) => `→ ${formatNumber(total)}`,
      newMark: "New",
      unpick: (label: string) => `Don't add ${label}`,
      dateLabel: (kind: "release" | "recorded" | "published") =>
        kind === "release" ? "Released" : kind === "recorded" ? "Recorded" : "Published",
      dateMode: (label: string) => `How to change ${label}`,
      dateNone: "No video has one",
      dateShared: (date: string, count: number, total: number) =>
        count === total
          ? `${date} on every video`
          : `${date} on ${formatNumber(count)} of ${videos(total)}`,
      dateMixed: (count: number, total: number) =>
        `Different dates on ${formatNumber(count)} of ${videos(total)}`,
      keep: "Keep",
      setDate: "Set",
      clear: "Clear",
      visibility: "Visibility",
      publicNow: (count: number, total: number) =>
        `${formatNumber(count)} of ${videos(total)} public`,
      public: "Public",
      private: "Private",
      changes: "Changes",
      noChanges: "Nothing will change yet.",
      changeAdd: (field: string, label: string, count: number) =>
        `${field}: add ${label} to ${videos(count)}`,
      changeRemove: (field: string, label: string, count: number) =>
        `${field}: remove ${label} from ${videos(count)}`,
      changeSet: (field: string, date: string, count: number) =>
        `${field}: set ${date} on ${videos(count)}`,
      changeClear: (field: string, count: number) =>
        `${field}: clear on ${videos(count)}`,
      changePublic: (count: number) => `Visibility: make ${videos(count)} public`,
      changePrivate: (count: number) => `Visibility: make ${videos(count)} private`,
      apply: (count: number) => `Apply to ${videos(count)}`,
      applying: "Applying…",
      applied: (count: number) => `Updated ${videos(count)}`,
      failed: (reason: string) => `Couldn't apply the changes: ${reason}`,
      discardTitle: "Discard these changes?",
      discardText: "The changes in the editor aren't applied yet.",
    },
    selection: {
      region: "Selection actions",
      count: (count: number) => `${videos(count)} selected`,
      countWithSmartFolders: (videoCount: number, folderCount: number) =>
        videoCount === 0
          ? `${smartFolders(folderCount)} selected`
          : `${videos(videoCount)} and ${smartFolders(folderCount)} selected`,
      smartFoldersUnsupported: "Not available for smart folders",
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
      edit: "Edit",
      unknownTarget: (id: number) => `#${formatNumber(id)}`,
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
      removeFromGroup: "Remove from group",
      deleteCollections: "Delete…",
      deleteNeedsCollection: "Select a group or smart folder",
      deleteDialog: {
        title: (groupCount: number, folderCount: number) =>
          `${collectionRemoval(groupCount, folderCount, "dissolve", "delete")}?`,
        groups: (count: number) =>
          selectPlural(count, {
            one: "The group is dissolved. Its videos and groups stay in the library.",
            other: `The ${formatNumber(count)} groups are dissolved. Their videos and groups stay in the library.`,
          }),
        smartFolders: (count: number) =>
          selectPlural(count, {
            one: "The smart folder is deleted with the smart folders inside it. Its videos stay in the library.",
            other: `The ${formatNumber(count)} smart folders are deleted with the smart folders inside them. Their videos stay in the library.`,
          }),
        submit: "Delete",
        submitting: "Deleting…",
        failed: (reason: string) => `Couldn't delete the selection: ${reason}`,
      },
      collectionsDeleted: (groupCount: number, folderCount: number) =>
        collectionRemoval(groupCount, folderCount, "dissolved", "deleted"),
      collectionsGone: "The selected groups and smart folders no longer existed",
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
    tabs: {
      label: "Tag lists",
      tags: "Tags",
    },
    columns: {
      name: "Name",
      videos: "Videos",
    },
    activeFilters: {
      label: "Active filters",
      remove: (name: string) => `Remove the filter "${name}"`,
    },
    loadFailed: "Couldn't load the tags",
    staleList: (reason: string) =>
      `Couldn't load tags: ${reason}. The list below may not match the current search, filters and sort.`,
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
    tentative: "Tentative",
    tentativeOnly: "Tentative only",
    tentativeOnlyHint: "Show only tags created by automatic tagging",
    noTentative: {
      title: "No tentative tags",
      description:
        "Tags created by automatic tagging appear here until you confirm or reject them.",
    },
    noTentativeMatches: (query: string) => `No tentative tags match "${query}"`,
    // Call zero-video tags Unused, not empty or orphan.
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
    // Kind names are user data and remain untranslated.
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
    kinds: {
      tab: "Kinds",
      table: "Kinds",
      description:
        "A tag has one kind or none. Set it from the Tags tab's selection bar. Video cards list tags of kinds marked First on cards before the rest. Deleting a kind leaves its tags without one.",
      newKind: "New kind",
      loading: "Loading kinds…",
      columns: {
        name: "Name",
        cardFirst: "First on cards",
        tags: "Tags",
      },
      tagCount,
      cardFirst: (name: string) => `List tags of the kind "${name}" first on video cards`,
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
    // Combine the submitted name with the API's original conflicting tagName.
    //
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
      // Avoid saying removed from 0 videos.
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
      noCandidates: "No matching tags",
      summarySources: (count: number) => tagCount(count),
      searchFailed: (reason: string) => `Couldn't search tags: ${reason}`,
      videoCount: videos,
      warning: (source: string, count: number, target: string) =>
        `The ${videos(count)} tagged "${source}" get the tag "${target}". "${source}" and its synonyms become synonyms of "${target}", and "${source}" leaves the tag list. This can't be undone.`,
      // count excludes the merge target; it is not the selection size. Avoid zero-video wording.
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
  credits: {
    person: creditKindTexts("person", "people", "People", "Person to merge into"),
    organization: creditKindTexts(
      "organization",
      "organizations",
      "Organizations",
      "Organization to merge into",
    ),
    loading: "Loading…",
    unnamedPerson: "Unnamed person",
    /** A provisional person's label: their id, short and unique where every one would read "Unnamed person". */
    personNumber: (id: number) => `#${String(id)}`,
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
    similarPeople: {
      loading: "Looking for people who look alike…",
      none: "No one looks alike yet.",
      failed: "Couldn't look for people who look alike.",
      likeFaces: "Looks most like these faces",
      likePerson: (name: string) => `Looks most like ${name}`,
      meta: (faces: number, firstSeen: string | undefined) => {
        const count = selectPlural(faces, {
          one: `${formatNumber(faces)} face`,
          other: `${formatNumber(faces)} faces`,
        });
        return firstSeen === undefined ? count : `${count} · first in ${firstSeen}`;
      },
      percent: (value: number) => `${formatNumber(Math.round(value * 100))}%`,
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
      personNote: "Faces found in videos for this person are deleted too.",
      submit: "Delete",
      submitting: "Deleting…",
    },
    deleted: (name: string) => `Deleted "${name}"`,
    peopleGrid: {
      youngestFirst: "Youngest first",
      oldestFirst: "Oldest first",
      noFavorites: "No favorite people yet",
      nameFilter: {
        label: "Name",
        any: "Any",
        named: "Named",
        unnamed: "Unnamed",
      },
      noUnnamed: "No unnamed people",
      noUnnamedDescription:
        "Unnamed people appear here when face recognition finds a face it doesn't know.",
      cardMeta: (age: string | null, count: number) =>
        age === null ? videos(count) : `${age} · ${videos(count)}`,
      newDialog: {
        title: "New person",
        submit: "Create",
        submitting: "Creating…",
        failed: (reason: string) => `Couldn't create the person: ${reason}`,
      },
    },
    personPage: {
      profile: "Person",
      loading: "Loading…",
      loadFailed: (reason: string) => `Couldn't load this person: ${reason}`,
      aliases: (list: readonly string[]) => `Also known as ${list.join(" · ")}`,
      edit: "Edit",
      name: "Name…",
      showInLibrary: "Show in library",
      more: "More actions",
      delete: "Delete",
      favoriteOff: "Add to favorites",
      favoriteOn: "Remove from favorites",
      groups: {
        profile: "Profile",
        measurements: "Measurements",
        career: "Career and bio",
      },
      noProfile: "No profile yet.",
      facts: {
        born: "Born",
        died: "Died",
        gender: "Gender",
        birthplace: "Birthplace",
        height: "Height",
        measurements: "Measurements",
        cup: "Cup",
        bloodType: "Blood type",
        career: "Career",
      },
      age: (age: string) => `age ${age}`,
      agedAtDeath: (age: string) => `aged ${age}`,
      ageRange: (low: number, high: number) =>
        low === high ? formatNumber(low) : `${formatNumber(low)}–${formatNumber(high)}`,
      dateWithAge: (date: string, age: string) => `${date} (${age})`,
      centimetres: (value: number) => `${formatNumber(value)} cm`,
      measurementsValue: (parts: readonly string[]) => `${parts.join("–")} cm`,
      measurementsShort: (parts: readonly string[]) => parts.join("–"),
      figures: {
        age: "Age",
        ageAtDeath: "Age at death",
        measurements: "Measurements (cm)",
        measurementsWithCup: (cup: string) => `Measurements (cm) · ${cup} cup`,
      },
      missingMeasurement: "–",
      careerRange: (from: number, until: number) => `${String(from)}–${String(until)}`,
      careerSince: (from: number) => `Since ${String(from)}`,
      careerUntil: (until: number) => `Until ${String(until)}`,
      genders: { female: "Female", male: "Male", other: "Other" },
      showMore: "Show more",
      showLess: "Show less",
      links: "Links",
      externalIds: "External IDs",
      externalId: (source: string, value: string) => `${source}: ${value}`,
      opensInNewTab: "(opens in a new tab)",
      face: {
        add: "Add a face",
        change: "Change face",
        upload: "Upload an image…",
        uploadTitle: "Upload a face",
        uploadHint:
          "PNG, JPEG or still WebP, up to 64 MB. You frame it before it is saved.",
        fromVideos: "Choose from videos…",
        reframe: "Reframe the face…",
        remove: "Remove face",
        refusedType: "This image can't be used. Choose a PNG, JPEG or still WebP.",
        tooLarge: (limit: string) => `This image is over ${limit}.`,
        failed: (reason: string) => `Couldn't save the face: ${reason}`,
        removeTitle: (name: string) => `Remove the face of "${name}"?`,
        removeText: "The image is deleted. You can upload another one later.",
        removeSubmit: "Remove",
        removing: "Removing…",
        crop: {
          title: "Frame the face",
          loading: "Reading the image…",
        },
        picker: {
          title: "Choose a face",
          close: "Close",
          faces: "Faces",
          facesHint: "The faces most like this person come first.",
          face: (title: string) => `Face in ${title}`,
          loading: "Loading faces…",
          noFaces: "No faces from videos yet. Run face recognition, then come back.",
          loadFailed: (reason: string) => `Couldn't load the faces: ${reason}`,
          frame: "Frame",
          frameLoading: "Loading the frame…",
          frameFailed: "Couldn't read this frame. Try another moment or face.",
          moment: (position: string) => `At ${position}`,
          earlier: "1 second earlier",
          later: "1 second later",
          size: "Size",
          sizeHint: "Drag the frame or use the arrow keys to move it.",
          area: "Face area",
          preview: "Preview",
          save: "Set as face",
          saving: "Saving…",
          cancel: "Cancel",
        },
      },
      videos: "Videos",
      roles: {
        performer: "Performer",
        director: "Director",
        photographer: "Photographer",
        streamer: "Streamer",
      },
      noVideos: "No videos yet",
      videosFailed: (reason: string) => `Couldn't load videos: ${reason}`,
      loadingMore: "Loading more videos…",
      moreFailed: "Couldn't load more videos",
      gone: "This person no longer exists",
      backToPeople: "Back to people",
      mergedInto: (name: string) => `Merged into "${name}"`,
      facesPage: {
        title: "Faces",
        entry: "Faces",
        toReview: (count: number) => `${formatNumber(count)} to review`,
        entryLabel: (count: number) =>
          count === 0 ? "Faces" : `Faces, ${formatNumber(count)} to review`,
        back: (name: string) => `Back to ${name}`,
        tabsLabel: "Face review",
        candidatesTab: "Same person?",
        facesTab: "Faces in videos",
        refresh: "Refresh",
        refreshHint: "Decided items stay where they are until you refresh.",
      },
      candidates: {
        heading: "Same person?",
        open: (name: string) => `Open ${name}`,
        similarity: (value: number) =>
          `${formatNumber(Math.round(value * 100))}% similar`,
        pairLabel: (name: string) => `Pair with ${name}`,
        percent: (value: number) => `${formatNumber(Math.round(value * 100))}%`,
        similarLabel: "similar",
        thisPerson: "This person",
        faceCount: (count: number) =>
          selectPlural(count, {
            one: `${formatNumber(count)} face`,
            other: `${formatNumber(count)} faces`,
          }),
        none: "No pairs to decide.",
        mergedResult: (name: string) => `Merged into ${name}`,
        dismissedResult: "Not the same",
        undo: "Undo",
        undoFor: (name: string) => `Undo for ${name}`,
        undoFailed: (reason: string) => `Couldn't undo: ${reason}`,
        confirm: "Same person",
        confirmFor: (name: string) => `Same person as ${name}`,
        dismiss: "Not the same",
        dismissFor: (name: string) => `Not the same as ${name}`,
        merged: (name: string) => `Merged "${name}"`,
        confirmFailed: (reason: string) => `Couldn't merge: ${reason}`,
        dismissFailed: (reason: string) => `Couldn't dismiss: ${reason}`,
        loadFailed: (reason: string) => `Couldn't load the pairs: ${reason}`,
      },
      faces: {
        heading: "Faces in videos",
        loading: "Loading the faces…",
        none: "No faces found in videos yet.",
        notInLibrary: "Not in the library",
        spanLink: (title: string, start: string) => `${title} from ${start}`,
        loadFailed: (reason: string) => `Couldn't load the faces: ${reason}`,
        loadingMore: "Loading more faces…",
        notThisPerson: "Not this person",
        notThisPersonFor: (title: string) => `Not this person in ${title}`,
        splitResult: (name: string) => `Split into ${name}`,
        undoSplitFor: (title: string) => `Undo the split in ${title}`,
        undoMoveFor: (title: string) => `Undo the move in ${title}`,
        splitFailed: (reason: string) => `Couldn't split the face: ${reason}`,
        undoSplitFailed: (reason: string) => `Couldn't undo the split: ${reason}`,
        sortLabel: "Order",
        sortLeastSimilar: "Least like this person first",
        sortNewest: "Newest video first",
        similarity: (value: number) => `${formatNumber(Math.round(value * 100))}%`,
        similarityFor: (title: string, value: number) =>
          `${title}, ${formatNumber(Math.round(value * 100))}% like this person`,
        select: "Select",
        selectFace: (title: string) => `Select ${title}`,
        selected: (count: number) => `${formatNumber(count)} selected`,
        selectedFull: (count: number) =>
          `${formatNumber(count)} selected, the most at once`,
        clearSelection: "Clear selection",
        splitSelected: "Not this person",
        moveSelected: "Move to person…",
        movedResult: (name: string) => `Moved to ${name}`,
      },
      moveDialog: {
        title: (count: number) =>
          selectPlural(count, {
            one: "Move 1 face to…",
            other: `Move ${formatNumber(count)} faces to…`,
          }),
        description: "Pick the person these faces belong to.",
        moving: "Moving",
        search: "Search by name",
        searchFailed: (reason: string) => `Couldn't search: ${reason}`,
        noCandidates: "No people match",
        summary: (count: number, name: string) =>
          selectPlural(count, {
            one: `1 face and its credit move to ${name}.`,
            other: `${formatNumber(count)} faces and their credits move to ${name}.`,
          }),
        submit: "Move",
        submitting: "Moving…",
        failed: (reason: string) => `Couldn't move the faces: ${reason}`,
        facesGone:
          "Some of these faces are no longer this person's. Refresh and try again.",
      },
      pluginFetch: {
        entry: "Fetch info…",
        title: (name: string) => `Fetch info for ${name}`,
        noAcceptedId: (plugin: string) =>
          `${plugin} has no ID for this person yet. Search by name instead.`,
        name: "Name",
        noMatches: "No matches. Try other words.",
        candidateId: (source: string, id: string) => `${source} ${id}`,
        candidateDetail: (detail: string, sourceId: string) => `${detail} · ${sourceId}`,
        source: (plugin: string, source: string, id: string) =>
          `From ${plugin} · ${source} ${id}`,
        sourceLink: (plugin: string, link: string) => `From ${plugin} · ${link}`,
        chooseLink: (plugin: string) =>
          `${plugin} has no ID for this person. Choose the link to fetch from.`,
        handledLinks: (plugin: string) => `Links ${plugin} handles`,
        linkTyped: "Fetches the person on this page instead of searching.",
        linkUnsupported: (plugin: string, host: string, domains: readonly string[]) =>
          `${plugin} doesn't handle links on ${host}. It accepts links from ${domains.join(", ")}.`,
        noLinkFetch: (plugin: string) =>
          `${plugin} doesn't fetch by link. Search by name instead.`,
        applied: (name: string) => `Applied to ${name}.`,
        gone: "This person no longer exists.",
        items: {
          image: "Face",
          "profile.birthDate": "Born",
          "profile.deathDate": "Died",
          "profile.gender": "Gender",
          "profile.birthplace": "Birthplace",
          "profile.heightCm": "Height",
          "profile.bustCm": "Bust",
          "profile.waistCm": "Waist",
          "profile.hipsCm": "Hips",
          "profile.cup": "Cup",
          "profile.bloodType": "Blood type",
          "profile.careerStartYear": "Career start",
          "profile.careerEndYear": "Career end",
          "profile.bio": "Bio",
          "profile.links": "Links",
          aliases: "Aliases",
          externalIds: "External IDs",
        } satisfies Record<PersonPluginItem, string>,
        empty: "Empty",
        onPerson: "On the person",
        sameNote: "Already the same as the fetched value.",
        searchByName: "Search by name",
        otherNames: "Also search for",
        back: "Back",
        face: "Face",
        keepFace: "Keep current",
        noFace: "No face",
        noNewFace: "No change",
        image: (n: number) => `Image ${formatNumber(n)}`,
        alreadyOnPerson: (count: number) =>
          `${formatNumber(count)} already on the person`,
        nothingNew: "Nothing new for this person.",
        skipped: {
          taken: (name: string) => `already linked to ${name}`,
          tooManyAliases: "over the limit for aliases",
          tooManyIds: "over the limit for external IDs",
          invalid: "too long or contains characters that can't be used",
        },
      },
    },
    personEdit: {
      title: (name: string) => `Edit "${name}"`,
      loadingTitle: "Edit",
      back: "Back to the person",
      profile: "Profile",
      namesAndLinks: "Names and links",
      gender: "Gender",
      notSet: "Not set",
      birthDate: "Birth date",
      deathDate: "Death date",
      dateHint: "YYYY-MM-DD, YYYY-MM or YYYY",
      birthplace: "Birthplace",
      bloodType: "Blood type",
      heightCm: "Height (cm)",
      bustCm: "Bust (cm)",
      waistCm: "Waist (cm)",
      hipsCm: "Hips (cm)",
      cup: "Cup",
      careerFrom: "Career from",
      careerUntil: "Career until",
      bio: "Bio",
      name: "Name",
      aliases: "Aliases",
      externalIds: "External IDs",
      url: "URL",
      meta: (parts: readonly string[]) => parts.join(" · "),
      saveFailed: (reason: string) => `Couldn't save the changes: ${reason}`,
      discardTitle: "Discard changes?",
      discardText: (name: string) => `Your changes to "${name}" aren't saved.`,
    },
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
      menuItemSelected: ", selected",
      transcoding: "Converting for playback",
      transcodingDetail:
        "The browser can't play this format directly, so it's converted while it plays. Seeking takes a few seconds.",
      // Quality names such as 480p remain untranslated.
      transcodingTo: (quality: string) => `Converting to ${quality}`,
      transcodingToDetail: (quality: string) =>
        `Playing a ${quality} version converted while it plays, so it needs less bandwidth. Choose “Original” in the quality menu to go back. Seeking takes a few seconds.`,
      quality: "Quality",
      qualityName: (quality: string) => quality,
      qualityOriginal: "Original",
      qualityOriginalOf: (quality: string) => `Original (${quality})`,
      qualityOriginalShort: "Orig",
      qualityNoSmaller: "No smaller sizes for this video",
    },
    subtitles: {
      button: "Subtitles",
      off: "Off",
      default: "Default",
    },
    // Chapter titles are user data and remain untranslated.
    chapters: {
      button: "Chapters",
      /** Untitled chapters use one-based chronological numbering. */
      untitled: (n: number) => `Chapter ${String(n)}`,
    },
    neighbors: {
      previous: "Previous video",
      next: "Next video",
      withTitle: (label: string, title: string) => `${label}: ${title}`,
    },
    loading: "Loading",
    reconnecting: "Connection lost · Reconnecting",
    // Report interruptions without suggesting a quality change.
    //
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
      earlier: (count: number) =>
        count === 1
          ? "Show 1 earlier video"
          : `Show ${formatNumber(count)} earlier videos`,
      moreFailed: "Couldn't load more of the group",
    },
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
      edited: "Edited in vv",
      created: "Created",
      dateDetail: (name: string, dateTime: string) => `${name} ${dateTime}`,
      openFile: "Open file",
      copyPath: "Copy path",
      pathCopied: "Copied the path",
      copyFailed: "Couldn't copy the path",
      openFailed: (reason: string) => `Couldn't open the file: ${reason}`,
      rescan: "Rescan",
      rescanNeedsMediaFolder: "Add a media folder in Settings first.",
      rescanStarted: "Started a force rescan of this video",
      thumbnailFailed: (reason: string) => `Couldn't change the thumbnail: ${reason}`,
      //
      favorite: "Favorite",
      favoriteFailed: (reason: string) => `Couldn't change the favorite: ${reason}`,
      technical: "Technical details",
      technicalPending: "Reading technical details…",
      technicalFailed: "Couldn't read the technical details",
    },
    tabs: {
      label: "About this video",
      overview: "Overview",
      work: "Work",
      people: "People",
      versions: "Versions",
      thumbnail: "Thumbnail",
      file: "File & media",
      resolution: "Resolution",
      lastWatched: "Last watched",
      released: (date: string) => `Released ${date}`,
      noWorkDetails: "No work details yet.",
      fetch: "Fetch",
      fetchAgain: "Fetch again",
      location: "Location",
      media: "Media",
      fileSection: "File",
      fileCreated: "File created",
      fileModified: "File modified",
      description: "Description",
      none: "None",
      chapters: "Chapters",
      container: "Container",
      chapterCount: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} chapter`,
          other: `${formatNumber(count)} chapters`,
        }),
      credits: "Credits",
      seenInVideo: "Seen in this video",
      seenHint: (length: string, pressable: boolean) =>
        pressable ? `0:00 — ${length} · press a bar to jump` : `0:00 — ${length}`,
      seenFailed: (reason: string) =>
        `Couldn't load the people seen in this video: ${reason}`,
      seenLoading: "Loading the people seen in this video",
      spanRange: (start: string, end: string) => `${start}–${end}`,
      spansOf: (name: string) => `When ${name} is on screen`,
      currentThumbnail: "Current",
      replaceWith: "Replace with",
      uploadedOrigin: "Uploaded image",
      generatedOrigin: "Generated image",
      automaticOrigin: "Automatic",
      backToAutomatic: "Back to automatic",
      frameOnScreen: "The frame on screen",
      useThisFrame: "Use this frame",
      anImage: "An image",
      uploadImage: "Upload…",
      uploadImageNote: "PNG, JPEG or WebP, up to 64 MB",
      touchUp: "Touch up with ComfyUI",
      touchUpButton: "Touch up…",
      touchUpNote: "Frame the image, then run an instruction",
      seekSprite: "Seek preview",
      seekSpriteLoading: "Loading the seek preview",
      seekSpriteFailed: "Couldn't load the seek preview",
      seekSpriteSheet: (index: number, count: number) =>
        `Seek preview sheet ${String(index)} of ${String(count)}`,
      openSeekSpriteSheet: (index: number, count: number) =>
        `Show seek preview sheet ${String(index)} of ${String(count)} large`,
    },
    //
    thumbnailEdit: {
      openSettings: "Open Settings",
      startFailed: (reason: string) => `Couldn't start editing the thumbnail: ${reason}`,
      title: "Edit thumbnail",
      frameAt: (time: string) => `Frame at ${time}`,
      //
      currentThumbnail: "Current thumbnail",
      thumbnailFrameAt: (time: string) => `Current thumbnail, frame at ${time}`,
      thumbnailSavedImage: "Current thumbnail, saved image",
      framingDetail: "Your framing",
      framing: "Framing",
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
      addReferenceTitle: "Add a reference image",
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
      presetsLabel: "Common edits",
      presets: {
        enhance: "Enhance quality",
        unblur: "Remove blur",
        removeText: "Remove text",
        fillEmpty: "Fill the empty space",
      },
    },
    //
    thumbnailUpload: {
      uploadTitle: "Upload a thumbnail",
      uploadHint: "PNG, JPEG or WebP, up to 64 MB. You frame it before it is used.",
      refusedType: "Choose a PNG, JPEG or WebP image.",
      refusedSize: "Choose an image up to 64 MB.",
      failed: (reason: string) => `Couldn't upload the image: ${reason}`,
      preview: {
        title: "Preview thumbnail",
        close: "Cancel and close",
        position: "Position",
        contain: "Whole image",
        cover: "Fill frame",
        moveHint: "Drag the image anywhere. Its edges and centre snap to the frame's.",
        unknownAspect:
          "The video's size is unknown, so the frame keeps the image's shape.",
        zoom: "Zoom",
        zoomValue: (percent: number) => `${formatNumber(percent)}%`,
        turn: "Turn",
        turnLeft: "Turn left",
        turnRight: "Turn right",
        backdrop: "Empty space",
        blur: "Blurred image",
        black: "Black",
        backdropHint: "Fills the parts of the frame the image doesn't cover.",
        stage: "Thumbnail preview. Drag or use the arrow keys to move the image.",
        apply: "Use as thumbnail",
        cancel: "Cancel",
      },
      ai: {
        title: "Touch up with ComfyUI",
        own: "Your own instruction",
        ownPlaceholder: "Or describe the change, for example: make it brighter",
        runOwn: "Run",
        hint: "Runs on the image as framed now. The result shows here before you use it.",
        starting: "Sending the image",
        stop: "Stop",
        cancelled: "Cancelled. The image is unchanged.",
        compare: "Before or after",
        before: "Before",
        after: "After",
        use: "Use result",
        dismiss: "Discard result",
        resultHint:
          "Using the result replaces the image, so you can frame it or touch it up again.",
        advanced: "Paint or add a reference…",
        setup: "Save a ComfyUI connection in Settings to touch up the image here.",
      },
    },
    versions: {
      count: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} version`,
          other: `${formatNumber(count)} versions`,
        }),
      label: "Versions",
      representative: "Representative",
      nowPlaying: "Now playing",
      playWord: "Play",
      play: (title: string, details: string) =>
        details === "" ? `Play ${title}` : `Play ${title}, ${details}`,
      makeRepresentative: "Make representative",
      remove: "Remove from versions",
      loadFailed: "Couldn't load the versions",
      retry: "Retry",
      changeFailed: (reason: string) => `Couldn't change the versions: ${reason}`,
      removed: (title: string) => `Removed "${title}" from the versions`,
    },
    title: {
      fileName: "File name",
      fileNameTitle: (name: string) => `File name: ${name}`,
    },
    // Use singular roles for one person and plural for multiple people.
    workInfo: {
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
      openPerson: (name: string) => `Open ${name}`,
      more: (count: number) => `+${formatNumber(count)} more`,
      showAll: (count: number, label: string) =>
        `Show all ${formatNumber(count)} ${label.toLowerCase()}`,
      showFewer: "Show fewer",
      showMore: "Show more",
      showLess: "Show less",
      edit: "Edit",
    },
    mediaInfo: {
      video: "Video",
      audio: "Audio",
      subtitles: "Subtitles",
      embedded: "Embedded",
      broadcast: "Broadcast",
      audioTracks: "Audio tracks",
      subtitleTracks: "Subtitle tracks",
      hdr: {
        sdr: "SDR",
        hdr10: "HDR10",
        hlg: "HLG",
        dolby_vision: "Dolby Vision",
      } satisfies Record<MediaInfoHdr, string>,
      frameRate: (rate: number) => `${formatDecimal(rate, 2)} fps`,
      bitDepth: (bits: number) => `${formatNumber(bits)}-bit`,
      bitRate: (bitsPerSecond: number) =>
        bitsPerSecond >= 1_000_000
          ? `${formatDecimal(bitsPerSecond / 1_000_000, 1)} Mbps`
          : `${formatNumber(Math.round(bitsPerSecond / 1000))} kbps`,
      interlaced: "Interlaced",
      spherical: "360°",
      stereo3d: (type: string) => `3D (${type})`,
      trackNumber: (n: number) => formatNumber(n),
      mono: "Mono",
      stereo: "Stereo",
      channels: (count: number) => `${formatNumber(count)} ch`,
      default: "Default",
      forced: "Forced",
      hearingImpaired: "Hearing impaired",
      closedCaptions: "Closed captions",
      title: "Title",
      author: "Author",
      comment: "Comment",
      description: "Description",
      genre: "Genre",
      date: "Date",
      recorded: "Recorded",
      device: "Device",
      location: "Location",
      coordinates: (latitude: number, longitude: number) =>
        `${formatDecimal(latitude, 4, 4)}, ${formatDecimal(longitude, 4, 4)}`,
      altitude: (metres: number) => `${formatNumber(Math.round(metres))} m`,
      coverArt: "Cover art",
      station: "Station",
      program: "Program",
    },
    workInfoFetch: {
      entry: "Fill in from a site",
      title: "Fetch work info",
      description: "Nothing changes until you apply.",
      plugin: "Plugin",
      querySearch: "Search words",
      queryFetch: "The work's ID",
      findBy: "Find by",
      findByOptions: {
        words: "Words",
        id: "ID",
      },
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
      noMatchesFetch:
        "No matches. Try other words, or choose ID and enter the work's ID.",
      source: (plugin: string) => `From ${plugin} · `,
      sourceId: (id: string) => `ID ${id}`,
      back: "Back to the matches",
      backToFind: "Back",
      now: "Now",
      fetched: "Fetched",
      fields: {
        title: "Title",
        tags: "Tags",
        cover: "Cover",
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
      nowThumbnail: "Current thumbnail",
      fetchedCover: (width: number, height: number) =>
        `Fetched cover, ${formatNumber(width)} × ${formatNumber(height)}. Becomes the thumbnail when you apply`,
      coverSize: (width: number, height: number) =>
        `${formatNumber(width)} × ${formatNumber(height)}`,
      adjustCover: "Adjust framing",
      keepThumbnail: "Keep current",
      coverOption: (n: number) => `Cover ${formatNumber(n)}`,
      credits: "People and organizations",
      sameNote: "Already the same as the fetched value.",
      onVideoCount: (count: number) => `${formatNumber(count)} already on the video`,
      samePeople: "Unnamed people",
      samePeopleNote:
        "Pick who each face on the video is. Applying merges the unnamed person into them.",
      samePersonLabel: (name: string) => `Who ${name} is`,
      keepApart: "Keep apart",
      fillsHint: "On the video; applying fills the person's empty fields",
      linkedTitle: "People linked to the video",
      linkedNote:
        "Fetch info on a person's page to fill the person from a person plugin.",
      fetchPersonInfo: (name: string) => `Fetch info for ${name}`,
      keepFraming: "Use this framing",
      nothingToAdd: "Nothing to add",
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
      fillsEmpty: (name: string, values: readonly string[]) =>
        `${name} — Fills only empty fields: ${values.join(" · ")}`,
      fillFact: (term: string, value: string) => `${term} ${value}`,
      fillBio: "Bio",
      // m counts selectable tags not already on the video.
      tagsSelected: (selected: number, selectable: number) =>
        `${formatNumber(selected)} of ${formatNumber(selectable)} selected`,
      more: (count: number) => `+${formatNumber(count)} more`,
      showFewer: "Show fewer",
      apply: "Apply selected",
      applying: "Applying…",
      applied: "Applied to the video.",
      resultTitle: "Applied, but some values were left out",
      skippedLabel: (label: string) => `${label}: `,
      skipped: {
        rejected: "you rejected this tag before",
        invalid: "not a valid tag name",
      } satisfies Record<SkippedTagReason, string>,
      skippedValue: {
        invalidDate: "not a date in the form YYYY-MM-DD, YYYY-MM or YYYY",
        invalidUrl: "not a full URL starting with http:// or https://",
        invalidOther: "too long or contains characters that can't be used",
        invalidCover: "couldn't be used as the thumbnail",
        tooMany: (items: string) => `over the limit for ${items}`,
        ambiguous: {
          person: "matches more than one person",
          organization: "matches more than one organization",
        },
      },
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
      unnamedPeople: "Unnamed people",
      addAs: (name: string) => `Add as ${name}`,
      unnamedRow: (label: string, count: number, hint: string | null) =>
        hint === null
          ? `${label}, ${videos(count)}`
          : `${label}, ${videos(count)}, ${hint}`,
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
    smartFolder: {
      line: (name: string, position: number, count: number) =>
        `Smart folder "${name}", video ${formatNumber(position)} of ${formatNumber(count)}. Open the smart folder`,
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
      rejudge: "Judge again",
      rejudgeFor: (first: string, second: string) =>
        `Judge again, for ${first} and ${second}`,
      rejudged: "Sent back to duplicate judging",
      left: "Left in the frames",
      right: "Right in the frames",
      noAudio: "No audio",
      moreAudio: (count: number) => `+${formatNumber(count)} more`,
      recommended: "Recommended representative",
      recommendedReason: {
        playable: "It plays in the browser as is; the other needs converting",
        audio: "It has audio and the other doesn't",
        audio_sync: "Its audio runs as long as its video; the other's doesn't",
        resolution: "It has the higher resolution",
        bitrate: "Same resolution, with more video data per second",
        older: "The files don't differ otherwise, so the one added first",
      },
      keepsData:
        "Bundling keeps both videos' tags, favourites, playback position and work information, whichever you choose.",
      hints: {
        unplayable: "Needs converting to play in the browser",
        encrypted: "Encrypted (DRM)",
        largerNoMorePixels: (times: number) =>
          `${formatDecimal(times, 1)}× the size without more pixels`,
        audioShare: (share: number) =>
          `Audio takes ${formatNumber(Math.round(share * 100))}% of the file`,
        audioLate: (ms: number) =>
          `Audio starts ${formatDecimal(ms / 1000, 1)} s after the video`,
        audioEarly: (ms: number) =>
          `Audio starts ${formatDecimal(ms / 1000, 1)} s before the video`,
        audioShorter: (ms: number) =>
          `Audio ends ${formatDecimal(ms / 1000, 1)} s before the video`,
        audioLonger: (ms: number) =>
          `Audio runs ${formatDecimal(ms / 1000, 1)} s past the video`,
      },
    },
    judgement: {
      heading: "Automatic judgement",
      why: "Why?",
      whyFor: (first: string, second: string) =>
        `Why the automatic judgement ended this way, for ${first} and ${second}`,
      whyTitle: "Why the automatic judgement ended this way",
      state: {
        waiting: "Waiting to be judged",
        running: "Being judged now",
        undecided: "Left to you: it couldn't tell whether these are the same video",
        undecidedAi: (percent: string) =>
          `Left to you: the AI was unsure, ${percent} sure both show the same picture`,
        undecidedIncomparable: "Left to you: the two videos' frames couldn't be compared",
        undecidedProperties:
          "Left to you: the AI says they differ, but the video information says they're the same",
        undecidedConflict:
          "Left to you: they look the same, but one is marked as a different video from the other's versions",
        failed: (detail: string) =>
          detail === "" ? "The judgement failed" : `The judgement failed: ${detail}`,
        no_connection: "Needs the AI connection to decide",
        held: "Left to you, since you changed an earlier automatic decision on this pair",
      },
      retryAt: (time: string) => `Tries again at ${time}`,
      judgingOff: "Duplicate judging is off, so this pair waits for you.",
      matched: (compared: number, differing: number) =>
        differing === 0
          ? `All ${formatNumber(compared)} frames compared by time show the same picture, so the rule judged them the same video without asking the AI.`
          : `${formatNumber(differing)} of ${formatNumber(compared)} frames compared by time differ, no two in a row and within the 5% allowed, so the rule judged them the same video without asking the AI.`,
      run: (
        differing: number,
        compared: number,
        run: number,
        start: string,
        end: string,
      ) =>
        `${formatNumber(differing)} of ${formatNumber(compared)} frames compared by time differ, ${formatNumber(run)} of them in a row from ${start} to ${end}. A section that differs throughout may be a replaced scene, an added intro or a different edit.`,
      scattered: (differing: number, compared: number) =>
        `${formatNumber(differing)} of ${formatNumber(compared)} frames compared by time differ here and there, more than the 5% allowed.`,
      versionMismatch:
        "The two fingerprints were made by different versions of vv, so the frames couldn't be compared. Rescan either video to compare them.",
      tooFew: (compared: number) =>
        `Only ${formatNumber(compared)} frames could be compared by time, fewer than the 3 needed, often because the picture is flat or dark.`,
      asked: (shown: number) =>
        selectPlural(shown, {
          one: `The AI was shown ${formatNumber(shown)} pair of those frames, below, and asked whether both sides show the same picture.`,
          other: `The AI was shown ${formatNumber(shown)} pairs of those frames, below, and asked whether both sides show the same picture.`,
        }),
      answer: (percent: string) =>
        `It answered ${percent}. At 80% or more the videos are bundled, at 20% or less they are marked different, and anything between is left to you.`,
      noAnswer: "The AI hasn't answered yet, so these frames are what it will be shown.",
      conflict:
        "The judgement found the same video, but one of these is already marked different from a video bundled with the other, so it was left to you.",
      byProperties:
        "The video information below settled this. Titled chapters at the same times or an equal recording time decide on their own; an equal embedded title or untitled chapters at the same times only tip an unsure AI answer; and an AI answer of different that they contradict is left to you.",
      properties: "Video information",
      propertyKind: {
        chapters: "Chapters",
        recorded_at: "Recorded at",
        embedded_title: "Embedded title",
        duration: "Length",
      },
      propertySame: "Same",
      propertyDifferent: "Different",
      propertyChapters: (count: string) =>
        selectPlural(Number(count), {
          one: `${formatNumber(Number(count))} chapter`,
          other: `${formatNumber(Number(count))} chapters`,
        }),
      propertyValues: (left: string, right: string) => `Left: ${left} · Right: ${right}`,
      frames: "Frames shown to the AI",
      frame: (left: string, right: string) => `Left at ${left}, right at ${right}`,
      frameAlt: (left: string, right: string) =>
        `Frame pair: left video at ${left}, right video at ${right}`,
    },
  },
  history: {
    title: "History",
    documentTitle: "History",
    list: "Watch history",
    day: {
      today: "Today",
      yesterday: "Yesterday",
      full: (name: string, date: string) => `${name}, ${date}`,
    },
    position: (position: string, duration: string) => `${position} / ${duration}`,
    resume: "Resume",
    resumeFor: (title: string) => `Resume ${title}`,
    startOver: "Start over",
    startOverFor: (title: string) => `Start ${title} over`,
    // Accessible names include the date, never the time.
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
  home: { title: "Home" },
  activity: {
    title: "Activity",
    documentTitle: "Activity",
    list: "Activity log",
    loading: "Loading activity",
    loadingMore: "Loading more activity",
    loadMoreFailed: "Couldn't load more activity",
    loadFailed: "Couldn't load the activity",
    search: {
      label: "Search file names",
    },
    filter: {
      status: "Status",
      feature: "Feature",
      decider: "Decided by",
      period: "Period",
      all: "All",
      clear: "Clear filters",
      apply: "Apply",
      several: (first: string, rest: number) => `${first} +${formatNumber(rest)}`,
      count: (count: number) => formatNumber(count),
    },
    statusValue: {
      running: "Running",
      attention: "Needs attention",
      done: "Done",
    },
    featureValue: {
      import: "Import",
      scan: "Scan",
      duplicate_check: "Duplicate check",
      auto_tagging: "Auto-tagging",
      work_info: "Work info",
      tags: "Tags",
      versions: "Versions",
      media_folders: "Media folders",
      organize: "Drag and drop",
      face_recognition: "Face recognition",
    } satisfies Record<ActivityFeature, string>,
    deciderValue: {
      ai: "AI",
      rule: "Rule",
      owner: "You",
    } satisfies Record<ActivityDecider, string>,
    periodValue: {
      any: "Any time",
      today: "Today",
      week: "Last 7 days",
      month: "Last 30 days",
    },
    statusIcon: {
      running: "Running",
      waiting: "Waiting",
      attention: "Needs attention",
      done: "Done",
      doneByYou: "Done by you",
    },
    kind: {
      import: "Import",
      scan: "Scan",
      version_judgement: "Duplicate check",
      auto_tag: "Auto-tagging",
      work_info_fetch: "Work info fetch",
      bundle: "Bundled",
      unbundle: "Removed from versions",
      make_representative: "Made representative",
      dismiss: "Marked different",
      tag_attach: "Tag added",
      tag_detach: "Tag removed",
      tag_keep: "Tag kept",
      tag_reject: "Tag rejected",
      work_info_apply: "Work info applied",
      media_folder_remove: "Media folder removed",
      organize_undo: "Drag and drop undone",
      face_analysis: "Face analysis",
      face_merge: "Person merged",
      pipeline: "Import run",
      undo: "Undo",
    } satisfies Record<ActivityKind, string>,
    subject: {
      pair: (first: string, second: string) => `${first} ↔ ${second}`,
      manualScan: "Manual scan",
      watchScan: "Watch scan",
    },
    stage: {
      probe: "Analyzing",
      thumbnail: "Thumbnail",
      seek_thumbnail: "Seek thumbnails",
      preview: "Preview",
      fingerprint: "Duplicate fingerprint",
    } satisfies Record<ActivityStage, string>,
    result: {
      waiting: "Waiting",
      running: "Running",
      done: "Done",
      progress: (done: number, total: number) =>
        `${formatNumber(done)}/${formatNumber(total)}`,
      importFailed: (stage: string, detail: string) =>
        detail === ""
          ? `Failed at ${stage.toLowerCase()}`
          : `Failed at ${stage.toLowerCase()}: ${detail}`,
      failed: (detail: string) => (detail === "" ? "Failed" : `Failed: ${detail}`),
      retryAt: (time: string) => `Failed · tries again at ${time}`,
      interrupted: "Stopped when vv shut down",
      noConnection: "Needs the AI connection to decide",
      undecided: "Couldn't tell whether these are the same video",
      undecidedAi: "AI",
      sameByAi: "Same video · bundled by AI",
      sameByRule: "Same video · bundled by rule",
      differentByAi: "Different videos by AI",
      differentByRule: "Different videos by rule",
      tagAdded: (name: string) => `+ ${name}`,
      tagRemoved: (name: string) => `− ${name}`,
      suggested: "(suggested)",
      noTags: "No tags added",
      tagsAdded: (count: number) =>
        selectPlural(count, {
          one: `Added ${formatNumber(count)} tag`,
          other: `Added ${formatNumber(count)} tags`,
        }),
      byYou: "by you",
      byYouAlone: "By you",
      folderRemoved: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} video left the library · by you`,
          other: `${formatNumber(count)} videos left the library · by you`,
        }),
      superseded: "Replaced by a newer run",
      decidedByOwner: "You decided on Duplicates",
      gone: "A video left the library",
      undid: (chip: string) => `Undid ${chip}`,
      undidTag: (name: string) => `Undid tag ${name}`,
      undidLink: (name: string) => `Undid the link to ${name}`,
      linked: (names: string) => `Linked ${names}`,
      createdPerson: (name: string) => `${name} (new)`,
      linksUndone: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} link undone`,
          other: `${formatNumber(count)} links undone`,
        }),
      allLinksUndone: "All links undone",
      noMainPerson: "No main person found",
      // A merged source is always provisional, so it is named by credits.unnamedPerson.
      mergedInto: (source: string, target: string) => `Merged "${source}" into ${target}`,
      personGone: "Deleted person",
      undoneSame: "Same video",
      undoneDifferent: "Different videos",
      alreadyReverted: "Already changed back by you",
      undone: "Undone by you",
      notInLibrary: "Not in the library",
    },
    percent: (value: number) => `${formatNumber(Math.round(value * 100))}%`,
    separator: "·",
    time: {
      justNow: "Just now",
      minutes: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} minute ago`,
          other: `${formatNumber(count)} minutes ago`,
        }),
      hours: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} hour ago`,
          other: `${formatNumber(count)} hours ago`,
        }),
      yesterday: (time: string) => `Yesterday ${time}`,
      at: (date: string, time: string) => `${date} ${time}`,
      shortNow: "now",
      shortMinutes: (count: number) => `${formatNumber(count)}m`,
      shortHours: (count: number) => `${formatNumber(count)}h`,
      shortYesterday: "Yesterday",
      // Use time only for today, date and time otherwise.
      retryAtDay: (date: string, time: string) => `${date}, ${time}`,
    },
    action: {
      open: "Open",
      openDuplicates: "Open in Duplicates",
      workInfo: "Work info",
      settings: "Settings",
      retry: "Retry",
      undo: "Undo",
      undoTags: "Undo tags…",
      undoLinks: "Undo links…",
      allLinks: "All links",
      openPerson: "Open person",
    },
    toast: {
      undone: "Undone. vv won't decide this again on its own",
      alreadyReverted: "Already changed back. vv won't decide this again on its own",
      retryFailed: (reason: string) => `Couldn't retry: ${reason}`,
      undoFailed: (reason: string) => `Couldn't undo: ${reason}`,
    },
    footer: {
      label: "Activity totals",
      running: (count: number) => `${formatNumber(count)} running`,
      attention: (count: number) => `${formatNumber(count)} need attention`,
      done: (count: number) => `${formatNumber(count)} done`,
    },
    empty: {
      title: "No activity yet",
      description:
        "Imports, auto-tagging, duplicate checks and your changes to tags and versions show up here.",
    },
    noMatch: {
      title: "Nothing matches these filters",
      description: "No entries match these filters.",
    },
    detailLink: (kind: string, subject: string) => `Details of ${kind} for ${subject}`,
    pipeline: {
      processing: (kind: string, done: number, total: number) =>
        selectPlural(total, {
          one: `${kind} in progress · ${formatNumber(done)} of ${formatNumber(total)} task done`,
          other: `${kind} in progress · ${formatNumber(done)} of ${formatNumber(total)} tasks done`,
        }),
      waiting: (done: number, total: number) =>
        selectPlural(total, {
          one: `Waiting · ${formatNumber(done)} of ${formatNumber(total)} task done`,
          other: `Waiting · ${formatNumber(done)} of ${formatNumber(total)} tasks done`,
        }),
      finished: (total: number) =>
        selectPlural(total, {
          one: `Finished · ${formatNumber(total)} task`,
          other: `Finished · ${formatNumber(total)} tasks`,
        }),
      matches: "Matches",
      showTasks: "Show tasks",
      hideTasks: "Hide tasks",
      showTasksFor: (subject: string) => `Show tasks for ${subject}`,
      hideTasksFor: (subject: string) => `Hide tasks for ${subject}`,
    },
    detail: {
      documentTitle: (subject: string) => `${subject} · Activity`,
      loading: "Loading this activity entry",
      notFound: "This activity entry doesn't exist",
      backToList: "Back to Activity",
      loadFailed: "Couldn't load this activity entry",
      section: {
        summary: "Summary",
        tasks: "Tasks",
        stages: "Stages",
        tags: "Tags",
        people: "People",
        partOf: "Part of",
        evidence: "Why",
      },
      fact: {
        status: "Status",
        process: "Process",
        video: "Video",
        videos: "Videos",
        person: "Person",
        decider: "Decided by",
        started: "Started",
        updated: "Updated",
        result: "Result",
      },
      deciderAi: (percent: string) => `AI ${percent}`,
      stageState: {
        waiting: "Waiting",
        processing: "In progress",
        done: "Done",
        failed: "Failed",
        gone: "Left the library",
        notRun: "Not run",
        absent: "Not part of this run",
      },
      stagesNotRecorded: "Stage details weren't recorded for this import.",
      pairMember: (label: string) => `Duplicate check · with ${label}`,
      undoTag: (name: string) => `Undo tag ${name}`,
      undoLink: (name: string) => `Undo link to ${name}`,
      newPerson: "New person",
      pipelineAttention: (kind: string, reason: string) => `${kind}: ${reason}`,
    },
  },
  playlists: {
    title: "Playlists",
    documentTitle: "Playlists",
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
    detail: {
      // Omit total duration if any item has an unknown duration.
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
  // Use conditions in the UI, not rule.
  smartFolders: {
    kind: "Smart folder",
    trail: "Smart folder path",
    save: "Save as smart folder",
    created: (name: string) => `Created "${name}"`,
    saved: (name: string) => `Saved "${name}"`,
    deleted: (name: string) => `Deleted "${name}"`,
    open: "Open",
    listFailed: (reason: string) => `Couldn't load the smart folders: ${reason}`,
    dialog: {
      newTitle: "New smart folder",
      newChildTitle: (parent: string) => `New smart folder in "${parent}"`,
      editTitle: "Edit smart folder",
      parent: "Parent",
      noParent: "None (top level)",
      sortOption: (kind: string, direction: string) => `${kind}, ${direction}`,
      ascending: "ascending",
      descending: "descending",
      randomSort: "Random can't be saved; the folder is sorted by date added",
      conditions: "Conditions",
      conditionsHint: "Videos that match every condition are in the folder.",
      noConditions: "Add at least one condition.",
      person: "Person",
      organization: "Organization",
      tagsLimit: (limit: number) => `Up to ${formatNumber(limit)} tags`,
      removeTag: (name: string) => `Remove the condition ${name}`,
      removePerson: (name: string) => `Remove the person ${name}`,
      removeOrganization: (name: string) => `Remove the organization ${name}`,
      removeKind: (name: string) => `Remove the kind ${name}`,
      removeDeleted: "Remove the deleted condition",
      submit: "Create",
      submitting: "Creating…",
      saveSubmit: "Save",
      saving: "Saving…",
      createFailed: (reason: string) => `Couldn't create the smart folder: ${reason}`,
      saveFailed: (reason: string) => `Couldn't save the smart folder: ${reason}`,
    },
    condition: {
      search: (query: string) => `Search: ${query}`,
      favorite: "Favorites",
      kind: (name: string) => `Kind: ${name}`,
      tag: (name: string) => `Tag: ${name}`,
      person: (name: string) => `Person: ${name}`,
      organization: (name: string) => `Organization: ${name}`,
      deleted: (kind: "tag" | "tagKind" | "person" | "organization") =>
        ({
          tag: "Tag: deleted",
          tagKind: "Kind: deleted",
          person: "Person: deleted",
          organization: "Organization: deleted",
        })[kind],
    },
    detail: {
      edit: "Edit…",
      newChild: "New child smart folder…",
      delete: "Delete…",
      editConditions: "Edit conditions…",
      conditions: "Conditions:",
      fromParent: (name: string) => `From "${name}":`,
      empty: {
        title: "Nothing matches these conditions yet",
        description: "Videos that match appear here as soon as they are tagged or added.",
      },
      missing: {
        title: "A condition no longer works",
        description:
          "A tag, kind, person or organization in the conditions was deleted, so nothing matches.",
        inParent: (name: string) => `The condition is in "${name}".`,
      },
      gone: {
        title: "This smart folder no longer exists",
        description: "It was deleted, maybe in another tab.",
      },
      loadFailed: "Couldn't load the smart folder",
      moreFailed: "Couldn't load more of the smart folder",
    },
    deleteDialog: {
      title: (name: string) => `Delete "${name}"?`,
      description: "The smart folder is deleted. Its videos stay in the library.",
      // Count descendants at every depth.
      descendants: (count: number) =>
        selectPlural(count, {
          one: "The 1 smart folder inside it is deleted with it.",
          other: `The ${formatNumber(count)} smart folders inside it are deleted with it.`,
        }),
      submit: "Delete",
      submitting: "Deleting…",
      failed: (reason: string) => `Couldn't delete the smart folder: ${reason}`,
    },
  },
  groups: {
    trail: "Group path",
    nameLabel: "Name",
    nameRule: "Use a name of 1 to 100 characters.",
    created: (name: string) => `Created "${name}"`,
    // A single selected group cannot open this dialog, so no singular-group conversion text is needed.
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
    add: {
      label: "Add to group",
      search: "Search or create a group",
      list: "Groups",
      create: (name: string) => `Create "${name}"`,
      none: "No groups yet. Type a name to create one.",
      nameTooLong: "Use a name of up to 100 characters",
      loading: "Loading…",
      loadFailed: "Couldn't load the groups",
      // Do not report a count: the server skips existing members.
      added: (name: string) => `Added to "${name}"`,
      failed: (reason: string) => `Couldn't add to the group: ${reason}`,
    },
    detail: {
      // Duration is preformatted; empty means unknown and shows only the count.
      count: (count: number, duration: string) =>
        duration === "" ? videos(count) : `${videos(count)} · ${duration}`,
      // The caller appends the folder path as a link.
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
  //
  organize: {
    drop: {
      putIn: "Put in",
      putInOrMerge: "Put in or merge…",
      refused: "Can't drop here",
      groupOrBundle: "Group or bundle…",
    },
    menu: {
      label: (name: string) => `Dropped on "${name}"`,
      makeGroup: "Make a group",
      nest: "Put in as a child group",
      merge: "Merge",
    },
    name: {
      label: "Group name",
      hint: (name: string) => `Enter keeps the name, Esc keeps "${name}"`,
    },
    done: {
      reordered: (name: string) => `Reordered "${name}"`,
      grouped: (name: string) => `Made the group "${name}"`,
      moved: (count: number, name: string) => `Moved ${items(count)} to "${name}"`,
      movedOut: (count: number, name: string) => `Moved ${items(count)} out of "${name}"`,
      // mixed includes merged videos; count items, not videos.
      merged: (count: number, name: string, mixed: boolean) =>
        `Merged ${mixed ? items(count) : groupsCount(count)} into "${name}"`,
    },
    failed: {
      reorder: (reason: string) => `Couldn't reorder: ${reason}`,
      group: (reason: string) => `Couldn't make the group: ${reason}`,
      move: (reason: string) => `Couldn't move: ${reason}`,
      merge: (reason: string) => `Couldn't merge: ${reason}`,
    },
    // Cards cannot be lifted by keyboard; keyboard users use the action menus.
    instructions:
      "Press and move a card to pick it up, then drop it on another card or a group to organize. With the keyboard, use the card's buttons instead.",
    undo: "Undo",
    undone: "Undone",
    undoFailed: {
      conflict: "Couldn't undo: it changed in the meantime",
      expired: "This can no longer be undone",
      other: (reason: string) => `Couldn't undo: ${reason}`,
    },
    // Positions are one-based.
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
  creditName: {
    required: "Enter a name",
    controlCharacters: "Line breaks and tabs aren't allowed",
    tooLong: (limit: number) => `Up to ${characters(limit)}`,
  },
  // オンボード（docs/design-docs/home.md「Words」の Onboarding の行）。
  onboarding: {
    documentTitle: "Set up your library",
    steps: "Setup steps",
    stepNames: {
      folders: "Media folders",
      scan: "Scan",
      interests: "Interests",
    },
    stepOf: (current: number, total: number) =>
      `Step ${formatNumber(current)} of ${formatNumber(total)}`,
    stepDone: (name: string) => `${name}, done`,
    stepCurrent: (name: string) => `${name}, current step`,
    skip: "Skip",
    saveFailed: (reason: string) => `Couldn't save your progress: ${reason}`,
    folders: {
      title: "Where are your videos?",
      description:
        "Add the folders on the server that hold your videos. You can change them later in Settings.",
      add: "Add folder",
      list: "Added folders",
      loading: "Loading the media folders",
      loadFailed: (reason: string) => `Couldn't load the media folders: ${reason}`,
      addFailed: (reason: string) => `Couldn't add the folder: ${reason}`,
      continue: "Continue",
    },
    scan: {
      title: "Scan your library",
      description:
        "VVMDM reads the folders and makes thumbnails. It keeps going in the background, so you can continue while it runs.",
      start: "Scan library",
      noFolder: "Add a media folder first.",
    },
    interests: {
      title: "What do you watch?",
      description:
        "Pick what your videos are about. Each becomes a smart folder in your library that fills in as videos get these tags.",
      label: "Interests",
      loading: "Loading the interests",
      loadFailed: (reason: string) => `Couldn't load the interests: ${reason}`,
      autoTaggingOff:
        "Auto-tagging is off, so these smart folders fill in only as you tag videos. You can turn it on in Settings.",
      hasFolder: "Already created",
      hasFolderLabel: (name: string) => `${name}, already created`,
      scanning: (settled: number, total: number) =>
        `Scanning: ${formatNumber(settled)} of ${formatNumber(total)}`,
      scanProgress: "Scan progress",
      scanFinished: "Scan finished",
      scanFailed: "Scan failed. See Settings.",
      finish: "Finish",
      pickFailed: (reason: string) => `Couldn't add the smart folders: ${reason}`,
      queueFailed: (reason: string) =>
        `Added the smart folders, but couldn't queue your videos for auto-tagging: ${reason}`,
      added: (count: number) =>
        selectPlural(count, {
          one: `Added ${formatNumber(count)} smart folder.`,
          other: `Added ${formatNumber(count)} smart folders.`,
        }),
    },
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
      home: "Home",
      library: "Library",
      folders: "Folders",
      tags: "Tags",
      people: "People",
      organizations: "Organizations",
      history: "History",
      playlists: "Playlists",
      duplicates: "Duplicates",
      activity: "Activity",
      activityCount: (count: number) =>
        selectPlural(count, {
          one: `${formatNumber(count)} needs attention`,
          other: `${formatNumber(count)} need attention`,
        }),
      railCount: (count: number) => (count > 99 ? "99+" : formatNumber(count)),
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
      rescanFailed: (reason: string) => `Couldn't start the force rescan: ${reason}`,
      rescanScope: {
        library: "Force rescan of the whole library",
        folder: "Force rescan of a media folder",
        namedFolder: (path: string) => `Force rescan of ${path}`,
        video: "Force rescan of one video",
      },
      notRun: "No scan has run yet",
      status: {
        starting: "Starting",
        finding: "Scanning",
        running: "Scanning",
        done: "Done",
        partial: "Some failed",
        failed: "Scan failed",
      },
      // The denominator includes changed files and unfinished videos; done includes failures.
      videosDone: (settled: number, total: number) =>
        `${formatNumber(settled)} of ${videos(total)} done`,
      videosShort: (settled: number, total: number) =>
        `${formatNumber(settled)} / ${formatNumber(total)}`,
      denominator:
        "Counts changed files and videos that still needed preparing, not the whole library.",
      noChanges: "No changed files were found.",
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
      location: (parts: readonly string[]) => parts.join(" / "),
      finishedAt: (when: string) => `Finished ${when}`,
      couldNotFinish: "The scan couldn't finish.",
      failedCount: (count: number) => `${formatNumber(count)} failed`,
      toCheckCount: (count: number) => `${formatNumber(count)} to check`,
      seeSettingsForList: "See Settings for the list.",
      // Announce only completion, partial failure, or failure milestones.
      // Automatic imports are silent while running, so only terminal states have copy.
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
    // Use the same label for navigation, h1 and browser title; keys match settingsPages.ts slugs.
    pages: {
      scan: "Scan",
      library: "Library",
      "auto-tagging": "Auto-tagging",
      "face-recognition": "Face recognition",
      connections: "Connections",
      plugins: "Plugins",
      conversion: "Video conversion",
      network: "Network",
      api: "API",
      "danger-zone": "Danger zone",
    },
    dangerZone: {
      clearPeople: {
        heading: "Delete all people",
        description:
          "Start the people of the library over: every person and every face found goes, and videos count as not yet analysed.",
        open: "Delete all people…",
        title: "Delete all people?",
        body: "Every person, named or not, is deleted with their faces, profiles and credits on videos, and every face found in videos goes too. Smart folders that name a person then show nothing. While face recognition is on, every video is analysed again; while it is off, turn it on and choose “Analyse videos not yet analysed”. This cannot be undone.",
        submit: "Delete all people",
        submitting: "Deleting…",
        done: (deleted: number, queued: number) =>
          `${selectPlural(deleted, {
            one: `${formatNumber(deleted)} person was deleted`,
            other: `${formatNumber(deleted)} people were deleted`,
          })}${
            queued === 0
              ? ""
              : selectPlural(queued, {
                  one: `; ${formatNumber(queued)} video was queued for face analysis`,
                  other: `; ${formatNumber(queued)} videos were queued for face analysis`,
                })
          }`,
        failed: (reason: string) => `Couldn't delete the people: ${reason}`,
      },
    },
    scanStatus: {
      heading: "Scan status",
      state: {
        notRun: "Not run",
        fetchFailed: "Rechecking",
      },
      rechecking: "Rechecking the latest status",
      start: "Scan library",
      starting: "Scanning…",
      runningReason: "A scan is running. You can start another when it finishes.",
      needsMediaFolder: { before: "Add a media folder in ", after: " to scan." },
      forceRescan: "Force rescan",
      rescanDialog: {
        title: "Force rescan the whole library?",
        description:
          "Every video is read again and its thumbnails, preview and duplicate check are rebuilt, even when the file hasn't changed. Auto-tagging and face analysis run again when they're on. This can take a long time.",
        submit: "Force rescan",
        starting: "Starting…",
      },
      issues: {
        heading: "Videos with problems",
        failed: "Failed",
        check: "Check",
        //
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
        "Files added, removed, moved or renamed in a media folder are picked up as they happen, without a scan. Changes made while it's off or while VVMDM isn't running are picked up by “Scan library” on the Scan page.",
      label: "Pick up changes as they happen",
      switchDescription: { before: "The watch state is on the ", after: " page." },
      // Insert the linked page name between before and after.
      state: {
        off: {
          before: "Off. Changes are picked up by the next scan. Turn it on in ",
          after: ".",
        },
        noFolder: { before: "Add a media folder in ", after: " to watch it." },
        starting: "Starting to watch the media folders…",
        active: "Watching the media folders.",
        limited: "Watching, but some changes may be missed.",
      },
      loading: "Loading the auto-import settings",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't change the setting: ${reason}`,
      loadFailed: (reason: string) => `Couldn't load the auto-import settings: ${reason}`,
      // Split around the user path so callers can render it as code.
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
      progressHeading: "Progress",
      off: "Auto-tagging is off.",
      offHint: "Turn on “Use auto-tagging” under Classifier to tag videos.",
      heading: "Classifier",
      description: {
        before: "Asks the classifier set in ",
        after:
          " which existing tags fit each video, from its thumbnail, frames from across the video, title, file name and folder. No new tags are created.",
      },
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
      lockTitle: "Ollama connection needed",
      lockBody: {
        before: "Save the Ollama URL and model in ",
        after: " to use auto-tagging.",
      },
      connectionReadFailed: (reason: string) =>
        `Couldn't read the Ollama connection: ${reason}`,
    },
    faceRecognition: {
      progressHeading: "Progress",
      off: "Face recognition is off.",
      offHint:
        "Turn on “Use face recognition” under Recognition to find people in videos.",
      queue: (queued: number, running: number, done: number, failed: number) =>
        `Waiting ${formatNumber(queued)} · Analysing ${formatNumber(running)} · Done ${formatNumber(done)} · Failed ${formatNumber(failed)}`,
      runMissing: "Analyse videos not yet analysed",
      started: (count: number) =>
        count === 0
          ? "There were no videos to analyse"
          : selectPlural(count, {
              one: `${formatNumber(count)} video was queued for face analysis`,
              other: `${formatNumber(count)} videos were queued for face analysis`,
            }),
      startFailed: (reason: string) => `Couldn't start face analysis: ${reason}`,
      lastFailure: "Last failure",
      heading: "Recognition",
      description:
        "Finds the main people of each video by their faces and links them to the video as performers. A face nobody is known for becomes an unnamed person, which you name on its page.",
      enabled: "Use face recognition",
      enabledHint:
        "Off: no video is analysed and nothing is downloaded. On: the face runtime and the chosen recognizer are downloaded once, and each imported video is analysed.",
      downloading: "Downloading the face runtime and recognizer…",
      downloadedBytes: (done: string, total: string) => `${done} of ${total}`,
      ready: "Ready",
      failedTitle: "Face recognition isn't ready",
      failedReason: {
        network: "The download couldn't finish. Check the connection and try again.",
        hash_mismatch:
          "A downloaded file didn't match what vv expected. Downloading again replaces it.",
        disk: "The files couldn't be written to the data folder.",
        helper_start: "The face runtime couldn't start.",
      },
      downloadAgain: "Download again",
      downloadFailed: (reason: string) => `Couldn't start the download: ${reason}`,
      unsupportedTitle: "Not available on this platform",
      unsupportedBody:
        "vv has no face runtime for this platform, so face recognition can't be turned on here.",
      recognizer: "Recognizer",
      recognizerHint:
        "Both find the same faces; the recognizer decides how faces are compared. Switching analyses every video again. Names you gave and merges you confirmed stay.",
      recognizerName: { sface_int8: "SFace", arcface_r50: "ArcFace r50" },
      nonCommercial: "Non-commercial",
      nonCommercialLicense: "Non-commercial use only",
      optionLine: (license: string, size: string) => `${license} · ${size}`,
      switchTitle: (name: string) => `Switch to ${name}?`,
      switchBody: (name: string, nonCommercial: boolean) =>
        `Every video is analysed again with ${name}, which takes a while. Names you gave and merges you confirmed stay.` +
        (nonCommercial ? ` ${name} is licensed for non-commercial use only.` : ""),
      switchAction: "Switch",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      loading: "Loading the face recognition settings",
      loadFailed: (reason: string) =>
        `Couldn't load the face recognition settings: ${reason}`,
    },
    versionJudging: {
      heading: "Duplicate judging",
      description: {
        before:
          "Judges each pair the scan lists as a possible duplicate: the two files are compared frame by frame, and the classifier set in ",
        after:
          " is asked only about the frames that differ. Same videos are bundled as versions, different videos leave Duplicates, and the rest wait there for you.",
      },
      label: "Judge possible duplicates automatically",
      hint: "Off: every possible duplicate waits on Duplicates for you. On: pairs that match throughout are bundled even without Ollama; pairs with a differing part wait until an Ollama connection is saved.",
      loading: "Loading the duplicate judging settings",
      saving: "Saving…",
      saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      loadFailed: (reason: string) =>
        `Couldn't load the duplicate judging settings: ${reason}`,
      connectionNote: {
        before:
          "No Ollama connection is saved. Pairs with a differing part wait until you save one in ",
        after: ".",
      },
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
      addedUnder: {
        work: (name: string) =>
          `Added ${name} under Work-info plugins. Approve it to use it.`,
        person: (name: string) =>
          `Added ${name} under Person plugins. Approve it to use it.`,
      } satisfies Record<WorkInfoPluginKind, (name: string) => string>,
      replacedUnder: {
        work: (name: string) =>
          `Replaced ${name} under Work-info plugins. Approve it again to use it.`,
        person: (name: string) =>
          `Replaced ${name} under Person plugins. Approve it again to use it.`,
      } satisfies Record<WorkInfoPluginKind, (name: string) => string>,
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
        fetch: "Fetch by ID",
      } satisfies Record<WorkInfoOperation, string>,
      person: {
        heading: "Person plugins",
        description:
          "Plugins fetch a person's face and profile from other sites, from the person page and when fetched work information names a person they know. Approve a plugin before it can run.",
        list: "Added person plugins",
        loading: "Loading the person plugins",
        loadFailed: (reason: string) => `Couldn't load the person plugins: ${reason}`,
        empty: "No person plugins yet",
        emptyHint: "Add a .wasm person plugin file to fetch faces and profiles.",
        operations: {
          search: "Search by name",
          fetch: "Fetch by ID",
        } satisfies Record<WorkInfoOperation, string>,
        accepts: (sources: readonly string[]) => `Accepts IDs from ${sources.join(", ")}`,
        acceptsLinks: (domains: readonly string[]) =>
          `Accepts links from ${domains.join(", ")}`,
      },
      network: "Reaches internet sites",
      settingSet: (label: string) => `${label} set`,
      settingNotSet: (label: string) => `${label} not set`,
      settingDefault: (label: string) => `${label} default`,
      settingsSummary: (items: readonly string[]) => `Settings: ${items.join(", ")}`,
      requiredMissing: (labels: readonly string[]) =>
        `Needs a value: ${labels.join(", ")}`,
      staleSettings: (labels: readonly string[]) => `Choose again: ${labels.join(", ")}`,
      defaultHint: (value: string) => `Default: ${value}`,
      lastRun: (when: string) => `Last run ${when}`,
      approve: "Review and approve",
      settings: "Settings",
      more: (name: string) => `More actions for ${name}`,
      revoke: "Revoke approval",
      lastRunOutput: "Last run output",
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
        personDescription:
          "Approved person plugins can run from a person page, and when fetched work information names a person with an ID they accept.",
        sources: "Accepts IDs from",
        domains: "Accepts links from",
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
        defaultItem: (value: string) => `Default (${value})`,
        notSetItem: "Not set",
        staleWithDefault: (value: string) =>
          `"${value}" isn't one of this version's choices. The default is used until you save.`,
        staleNoDefault: (value: string) =>
          `"${value}" isn't one of this version's choices. No value is sent until you save.`,
        submit: "Save",
        saved: (name: string) => `Saved the settings for ${name}.`,
        saveFailed: (reason: string) => `Couldn't save the settings: ${reason}`,
      },
      runDialog: {
        title: (name: string) => `Last run of ${name}`,
        description:
          "What the plugin wrote to its error output during its last run. Values of secret settings are shown as [secret].",
        ran: "Ran",
        did: "Did",
        result: "Result",
        finished: "Finished",
        failed: (reason: string) => `Failed: ${reason}`,
        truncated: "The output was cut at 64 KB.",
        output: "Output",
        wroteNothing: "The last run wrote nothing.",
        noRun: "No run yet. Run the plugin from a video page, then open this again.",
        loading: "Loading the last run",
        loadFailed: (reason: string) => `Couldn't load the last run: ${reason}`,
        close: "Close",
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
    setup: {
      heading: "Setup",
      setup: "Setup",
      setupDescription: "Add media folders, scan and pick interests again.",
      startSetup: "Start setup",
      setupFailed: (reason: string) => `Couldn't start setup: ${reason}`,
    },
    mediaFolders: {
      heading: "Media folders",
      description:
        "Folders on the server where VVMDM looks for videos. After adding or changing a folder, start a scan with “Scan library” on the Scan page. Auto-import doesn't start one.",
      lockedWhileScanning: "You can't change media folders while a scan is running",
      rowsLockedWhileScanning:
        "You can't change or force rescan media folders while a scan is running",
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
      forceRescan: "Force rescan folder",
      forceRescanShort: "Force rescan",
      rescanDialog: {
        title: "Force rescan this folder?",
        description:
          "Every video in this folder is read again and its thumbnails, preview and duplicate check are rebuilt, even when the file hasn't changed. Auto-tagging and face analysis run again when they're on. This can take a long time.",
        target: "Folder",
      },
    },
    transcoding: {
      heading: "Encoder",
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
      heading: "Local network",
      notDesktop:
        "LAN access is set in the Windows desktop app. This VVMDM isn't running as the desktop app, so there's no switch here.",
      description: "Connections to VVMDM, and from its plugins to other sites.",
      loading: "Loading the network settings",
      loadFailed: (reason: string) => `Couldn't load the network settings: ${reason}`,
      lanAccess: "Allow connections from the local network",
      lanAccessDescription:
        "By default only this PC can open VVMDM. Allow connections from your local network to open it from a phone or another computer on the same network.",
      proxy: {
        label: "Proxy for plugin requests",
        description:
          "Plugin requests to other sites go through this proxy. Leave it empty to connect directly. Devices on your network are refused either way.",
        placeholder: "http://proxy.example:3128",
        invalid:
          "Enter a URL starting with http://, https:// or socks5://, with a host and no path.",
        saved: "Saved.",
        saveFailed: (reason: string) => `Couldn't save the proxy: ${reason}`,
      },
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
