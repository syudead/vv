import { reloadPage } from "../auth/pageNavigation";
import type { components, operations } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { nextProgressSequence, recordSavedProgress } from "./progressEvents";

export type Video = components["schemas"]["Video"];
export type SeekThumbnailSprite = components["schemas"]["SeekThumbnailSprite"];
export type VideoPage = components["schemas"]["VideoPage"];
export type VideoSort = components["schemas"]["VideoSort"];

export const videoSorts: readonly VideoSort[] = [
  "addedAsc",
  "addedDesc",
  "modifiedAsc",
  "modifiedDesc",
  "createdAsc",
  "createdDesc",
  "titleAsc",
  "titleDesc",
  "durationAsc",
  "durationDesc",
  "sizeAsc",
  "sizeDesc",
  "playedAsc",
  "playedDesc",
  "favoritedAsc",
  "favoritedDesc",
  "random",
];

export function isVideoSort(value: unknown): value is VideoSort {
  return typeof value === "string" && (videoSorts as readonly string[]).includes(value);
}
export type TranscodeQuality = NonNullable<
  NonNullable<operations["transcodeVideo"]["parameters"]["query"]>["quality"]
>;

export const transcodeQualities: readonly TranscodeQuality[] = [
  "1080p",
  "720p",
  "480p",
  "360p",
];

export function isTranscodeQuality(value: unknown): value is TranscodeQuality {
  return (
    typeof value === "string" && (transcodeQualities as readonly string[]).includes(value)
  );
}
export type WatchFilter = components["schemas"]["WatchFilter"];
export type TagRef = components["schemas"]["TagRef"];
export type VideoTag = components["schemas"]["VideoTag"];
export type FolderScope = components["schemas"]["FolderScope"];
export type VideoFolder = components["schemas"]["VideoFolder"];
export type Scan = components["schemas"]["Scan"];
export type ScanActivity = components["schemas"]["ScanActivity"];
export type ScanActivityKind = components["schemas"]["ScanActivityKind"];
export type ScanIssue = components["schemas"]["ScanIssue"];
export type ScanIssueKind = components["schemas"]["ScanIssueKind"];
export type ScanIssuePage = components["schemas"]["ScanIssuePage"];
export type Progress = components["schemas"]["Progress"];
type ProgressUpdate = components["schemas"]["ProgressUpdate"];
export type TranscodeStart = components["schemas"]["TranscodeStart"];
export type SubtitleTrack = components["schemas"]["SubtitleTrack"];
export type SubtitleTrackList = components["schemas"]["SubtitleTrackList"];
export type MediaFolder = components["schemas"]["MediaFolder"];
export type DirectoryListing = components["schemas"]["DirectoryListing"];
export type ApiError = components["schemas"]["Error"];
export type FolderSummary = components["schemas"]["FolderSummary"];
export type FolderPreview = components["schemas"]["FolderPreview"];
export type FolderListing = components["schemas"]["FolderListing"];
export type RootFolderListing = components["schemas"]["RootFolderListing"];
export type RelatedVideos = components["schemas"]["RelatedVideos"];
export type GroupMemberPage = components["schemas"]["GroupMemberPage"];
export type VideoLocation = components["schemas"]["VideoLocation"];
export type VideoVersions = components["schemas"]["VideoVersions"];
export type VideoVersionsRef = components["schemas"]["VideoVersionsRef"];
export type VersionCandidate = components["schemas"]["VersionCandidate"];
export type VersionCandidatePage = components["schemas"]["VersionCandidatePage"];
export type LibraryGroup = components["schemas"]["LibraryGroup"];
export type TranscodingSettings = components["schemas"]["TranscodingSettings"];
export type NetworkSettings = components["schemas"]["NetworkSettings"];
export type PluginProxySettings = components["schemas"]["PluginProxySettings"];
export type AutoImportSettings = components["schemas"]["AutoImportSettings"];
export type VersionJudgingSettings = components["schemas"]["VersionJudgingSettings"];
export type AutoTaggingSettings = components["schemas"]["AutoTaggingSettings"];
export type CheckResult = components["schemas"]["CheckResult"];
export type OllamaSettings = components["schemas"]["OllamaSettings"];
export type UpdateOllamaSettings = components["schemas"]["UpdateOllamaSettingsRequest"];
export type ComfyUISettings = components["schemas"]["ComfyUISettings"];
export type UpdateComfyUISettings = components["schemas"]["UpdateComfyUISettingsRequest"];
export type ComfyUIMissingItem = components["schemas"]["ComfyUIMissingItem"];
export type ThumbnailEditReadiness = components["schemas"]["ThumbnailEditReadiness"];
export type ThumbnailEdit = components["schemas"]["ThumbnailEdit"];
export type ThumbnailEditState = components["schemas"]["ThumbnailEditState"];
export type ThumbnailEditFailure = components["schemas"]["ThumbnailEditFailure"];
export type AutoTaggingScope = components["schemas"]["StartAutoTaggingRequest"]["scope"];
export type UpdateAutoTaggingSettings =
  components["schemas"]["UpdateAutoTaggingSettingsRequest"];
export type FolderWatch = components["schemas"]["FolderWatch"];
export type FaceRecognitionSettings = components["schemas"]["FaceRecognitionSettings"];
export type FaceRecognizer = components["schemas"]["FaceRecognizer"];
export type FaceRecognitionScope =
  components["schemas"]["StartFaceRecognitionRequest"]["scope"];
export type VideoEncoderChoice = components["schemas"]["VideoEncoderChoice"];
export type VideoEncoder = components["schemas"]["VideoEncoder"];
export type EncoderAvailability = components["schemas"]["EncoderAvailability"];
export type EncoderUnavailableReason = components["schemas"]["EncoderUnavailableReason"];
export type APIToken = components["schemas"]["APIToken"];
export type CreatedAPIToken = components["schemas"]["CreatedAPIToken"];

/** Card values are fetched separately when a smart-folder card becomes visible. */
export type SmartFolderSummary = components["schemas"]["SmartFolderSummary"];

export type LibraryItemOf<G> =
  | { kind: "video"; video: Video }
  | { kind: "group"; group: G }
  | { kind: "smartFolder"; smartFolder: SmartFolderSummary };

export type LibraryItem = LibraryItemOf<LibraryGroup>;
export type VideoChanged = components["schemas"]["VideoChanged"];
export type GroupChanged = components["schemas"]["GroupChanged"];
export type GroupDetail = components["schemas"]["GroupDetail"];

// Match the contract's default page size; the initial screen waits for one page only.
export const PAGE_SIZE = 60;
export const MAX_PAGE_SIZE = 200;

export type ErrorCode = ApiError["code"];
export type ErrorReason = components["schemas"]["ErrorReason"];
export type ProbeErrorCode = components["schemas"]["ProbeErrorCode"];
export type ScanErrorCode = components["schemas"]["ScanErrorCode"];
export type StartRescanRequest = components["schemas"]["StartRescanRequest"];

export interface RequestFailedDetails {
  reason?: string;
  limit?: number;
  tagName?: string;
  /** Server field path, such as code or dates[0].date. */
  field?: string;
  /** First 300 characters of the plugin error message. */
  pluginMessage?: string;
  host?: string;
  settingKeys?: readonly string[];
}

/** Localize known error codes; use the server's English message only for unknown codes. Preserve status and code, or an empty code for non-JSON bodies. */
export class RequestFailed extends Error {
  readonly status: number;
  readonly code: string;
  readonly reason: string | undefined;
  readonly limit: number | undefined;
  readonly tagName: string | undefined;
  readonly field: string | undefined;
  readonly pluginMessage: string | undefined;
  readonly host: string | undefined;
  readonly settingKeys: readonly string[] | undefined;

  constructor(
    status: number,
    code: string,
    message: string,
    details: RequestFailedDetails = {},
  ) {
    super(message);
    this.name = "RequestFailed";
    this.status = status;
    this.code = code;
    this.reason = details.reason;
    this.limit = details.limit;
    this.tagName = details.tagName;
    this.field = details.field;
    this.pluginMessage = details.pluginMessage;
    this.host = details.host;
    this.settingKeys = details.settingKeys;
  }
}

/** Fetch-level failure; browser-specific messages are not user-facing. */
export class NetworkFailed extends Error {
  constructor(cause: unknown) {
    super("network request failed", { cause });
    this.name = "NetworkFailed";
  }
}

/** Route API requests through this boundary; wrap fetch failures but preserve AbortError. */
export async function sendRequest(path: string, init?: RequestInit): Promise<Response> {
  try {
    return await fetch(path, init);
  } catch (error) {
    if (isAborted(error)) throw error;
    throw new NetworkFailed(error);
  }
}

let renderedAudience: "owner" | "guest" | null = null;
let reloading = false;

/** Track the gated audience so API responses can detect loss of owner access. */
export function setRenderedAudience(audience: "owner" | "guest" | null): void {
  renderedAudience = audience;
  reloading = false;
}

/** Reload the current URL once so the auth gate rechecks the audience. */
export function reloadForViewerChange(): void {
  if (reloading) return;
  reloading = true;
  reloadPage();
}

/** EventSource and media errors cannot expose response headers; use an explicit session check to detect audience changes. */
export function reloadIfNoLongerOwner(state: string): boolean {
  if (renderedAudience !== "owner" || state === "owner") return false;
  reloadForViewerChange();
  return true;
}

function viewerChanged(response: Response): boolean {
  if (renderedAudience !== "owner") return false;
  return response.status === 401 || response.headers.get("X-VV-Audience") === "guest";
}

let writes = 0;

/** Counts write requests sent through apiFetch, so a kept list can tell it may be out of date. */
export function apiWriteCount(): number {
  return writes;
}

/** On owner-to-guest responses, reload once and never resolve, preventing guest data from reaching an owner view. Auth routes bypass this wrapper. */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const method = init?.method?.toUpperCase() ?? "GET";
  const write = method !== "GET" && method !== "HEAD";
  if (write) writes += 1;
  let response: Response;
  try {
    response = await sendRequest(path, init);
  } finally {
    // Count the write again when it settles, so a list kept while it was in flight is dropped too.
    if (write) writes += 1;
  }
  if (viewerChanged(response)) {
    reloadForViewerChange();
    return new Promise<Response>(() => undefined);
  }
  return response;
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await apiFetch(path, init);

  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  return (await response.json()) as T;
}

function stringArray(value: unknown): readonly string[] | undefined {
  return Array.isArray(value) && value.every((item) => typeof item === "string")
    ? (value as string[])
    : undefined;
}

export async function toRequestFailed(response: Response): Promise<RequestFailed> {
  let body: Partial<ApiError> | null = null;
  try {
    body = (await response.json()) as Partial<ApiError> | null;
  } catch {
    // Proxies and incorrect routes may return non-JSON error bodies.
  }
  if (typeof body !== "object" || body === null) {
    return new RequestFailed(response.status, "", "");
  }
  return new RequestFailed(
    response.status,
    typeof body.code === "string" ? body.code : "",
    typeof body.message === "string" ? body.message : "",
    {
      reason: typeof body.reason === "string" ? body.reason : undefined,
      limit: typeof body.limit === "number" ? body.limit : undefined,
      tagName: typeof body.tagName === "string" ? body.tagName : undefined,
      field: typeof body.field === "string" ? body.field : undefined,
      pluginMessage:
        typeof body.pluginMessage === "string" ? body.pluginMessage : undefined,
      host: typeof body.host === "string" ? body.host : undefined,
      settingKeys: stringArray(body.settingKeys),
    },
  );
}

export const MAX_QUERY_LENGTH = 100;

export interface ListFilterParams {
  /** Omit empty queries and truncate to 100 Unicode code points. */
  query?: string;
  /** Omitted values use the server's all default. */
  watch?: WatchFilter;
  /** Send true only; false uses the server default. */
  playable?: boolean;
  /** Send true only; false uses the server default. */
  favorite?: boolean;
  sort?: VideoSort;
  /** Random-sort seed in 0..2147483647. */
  seed?: number;
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

export const MAX_TAG_FILTER_COUNT = 16;

/** Tag filtering applies to the library, not folder-video listings. */
export interface ListVideosParams
  extends ListFilterParams, CreditFilterParams, AddedLengthFilterParams {
  /** AND up to 16 tag IDs; order is irrelevant. */
  tag?: number[];
}

/** Match linked people or organizations in any role. */
export interface CreditFilterParams {
  person?: number;
  /** Owner-only; keeps videos where person has this role and requires person. */
  personRole?: components["schemas"]["WorkPersonRole"];
  organization?: number;
  /** Match any manual or folder-derived tag of this kind. */
  tagKind?: number;
}

function setCreditFilter(query: URLSearchParams, params: CreditFilterParams): void {
  if (params.person !== undefined) query.set("person", String(params.person));
  if (params.personRole !== undefined) query.set("personRole", params.personRole);
  if (params.organization !== undefined) {
    query.set("organization", String(params.organization));
  }
  if (params.tagKind !== undefined) query.set("tagKind", String(params.tagKind));
}

/** Send bounds unchanged; the server rejects out-of-range values instead of clamping them. */
export interface AddedLengthFilterParams {
  /** Inclusive age bound in days, 1..36500. */
  addedWithin?: number;
  /** Inclusive minimum seconds, 0..360000; unknown durations do not match. */
  minDuration?: number;
  /** Inclusive maximum seconds, 0..360000; unknown durations do not match. */
  maxDuration?: number;
}

function setAddedLengthFilter(
  query: URLSearchParams,
  params: AddedLengthFilterParams,
): void {
  if (params.addedWithin !== undefined)
    query.set("addedWithin", String(params.addedWithin));
  if (params.minDuration !== undefined)
    query.set("minDuration", String(params.minDuration));
  if (params.maxDuration !== undefined)
    query.set("maxDuration", String(params.maxDuration));
}

function setListFilters(query: URLSearchParams, params: ListFilterParams): void {
  if (params.query !== undefined && params.query !== "") {
    // Count Unicode code points like the server without splitting surrogate pairs.
    query.set("query", Array.from(params.query).slice(0, MAX_QUERY_LENGTH).join(""));
  }
  if (params.watch !== undefined) query.set("watch", params.watch);
  if (params.playable === true) query.set("playable", "true");
  if (params.favorite === true) query.set("favorite", "true");
  if (params.sort !== undefined) query.set("sort", params.sort);
  if (params.seed !== undefined) query.set("seed", String(params.seed));
  if (params.cursor !== undefined && params.cursor !== "") {
    query.set("cursor", params.cursor);
  }
  query.set("limit", String(params.limit ?? PAGE_SIZE));
}

function setTagFilter(query: URLSearchParams, tag?: number[]): void {
  if (tag === undefined) return;
  for (const id of tag.slice(0, MAX_TAG_FILTER_COUNT)) {
    query.append("tag", String(id));
  }
}

export function listVideos(params: ListVideosParams = {}): Promise<VideoPage> {
  const query = new URLSearchParams();
  setListFilters(query, params);
  setTagFilter(query, params.tag);
  setCreditFilter(query, params);
  setAddedLengthFilter(query, params);
  return request<VideoPage>(`/api/videos?${query.toString()}`, { signal: params.signal });
}

export type LibraryPageResponse = Omit<components["schemas"]["LibraryPage"], "items"> & {
  items: LibraryItem[];
};

export function toLibraryItem<G>(item: {
  kind: LibraryItem["kind"];
  video?: Video;
  group?: G;
  smartFolder?: SmartFolderSummary;
}): LibraryItemOf<G> | null {
  if (item.kind === "video" && item.video !== undefined) {
    return { kind: "video", video: item.video };
  }
  if (item.kind === "group" && item.group !== undefined) {
    return { kind: "group", group: item.group };
  }
  if (item.kind === "smartFolder" && item.smartFolder !== undefined) {
    return { kind: "smartFolder", smartFolder: item.smartFolder };
  }
  return null;
}

export function toLibraryItems<G>(
  items: readonly Parameters<typeof toLibraryItem<G>>[0][],
): LibraryItemOf<G>[] {
  return items.flatMap((item) => {
    const converted = toLibraryItem(item);
    return converted === null ? [] : [converted];
  });
}

export type ListLibraryParams = ListVideosParams;

export async function listLibrary(
  params: ListLibraryParams = {},
): Promise<LibraryPageResponse> {
  const query = new URLSearchParams();
  setListFilters(query, params);
  setTagFilter(query, params.tag);
  setCreditFilter(query, params);
  setAddedLengthFilter(query, params);
  const page = await request<components["schemas"]["LibraryPage"]>(
    `/api/library?${query.toString()}`,
    { signal: params.signal },
  );
  return { ...page, items: toLibraryItems(page.items) };
}

/** Registered root ID plus a slash-separated relative path. */
export interface FolderRef {
  rootId: number;
  /** Empty for the registered root itself. */
  path: string;
}

export function listRootFolders(signal?: AbortSignal): Promise<RootFolderListing> {
  return request<RootFolderListing>("/api/folders", { signal });
}

function folderQuery(folder: FolderRef): URLSearchParams {
  const query = new URLSearchParams();
  if (folder.path !== "") query.set("path", folder.path);
  return query;
}

export function getFolder(
  folder: FolderRef,
  signal?: AbortSignal,
): Promise<FolderListing> {
  const query = folderQuery(folder);
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return request<FolderListing>(`/api/folders/${String(folder.rootId)}${suffix}`, {
    signal,
  });
}

export interface ListFolderVideosParams extends ListFilterParams {
  folder: FolderRef;
  /** direct includes immediate children; subtree includes descendants. Omit for the server default. */
  scope?: FolderScope;
}

export function listFolderVideos(params: ListFolderVideosParams): Promise<VideoPage> {
  const query = folderQuery(params.folder);
  if (params.scope !== undefined) query.set("scope", params.scope);
  setListFilters(query, params);
  return request<VideoPage>(
    `/api/folders/${String(params.folder.rootId)}/videos?${query.toString()}`,
    { signal: params.signal },
  );
}

/** Read all descendant videos regardless of current filters; missing or invisible groups return 404. */
export function getGroup(id: number, signal?: AbortSignal): Promise<GroupDetail> {
  return request<GroupDetail>(`/api/groups/${String(id)}`, { signal });
}

function groupQuery(
  params: Record<string, string>,
  group: number | undefined,
  smartFolder: number | undefined,
): string {
  const query = new URLSearchParams(params);
  if (group !== undefined) query.set("group", String(group));
  if (smartFolder !== undefined) query.set("smartFolder", String(smartFolder));
  return query.size === 0 ? "" : `?${query.toString()}`;
}

/** Group context takes precedence over smart-folder context when choosing playback order. */
export function getVideo(
  id: number,
  signal?: AbortSignal,
  group?: number,
  smartFolder?: number,
): Promise<Video> {
  return request<Video>(`/api/videos/${id}${groupQuery({}, group, smartFolder)}`, {
    signal,
  });
}

/** Clear whitespace-only names and invalidate snapshots because detached lists cannot observe title/search/order changes. */
export async function setVideoDisplayName(
  id: number,
  displayName: string,
  signal?: AbortSignal,
): Promise<Video> {
  const video = await request<Video>(`/api/videos/${String(id)}/display-name`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ displayName }),
    signal,
  });
  clearListSnapshot();
  return video;
}

/** Null restores automatic positioning; return after generation completes and invalidate detached list thumbnails. */
export async function setVideoThumbnailPosition(
  id: number,
  positionMs: number | null,
  signal?: AbortSignal,
): Promise<Video> {
  const video = await request<Video>(`/api/videos/${String(id)}/thumbnail-position`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ positionMs }),
    signal,
  });
  clearListSnapshot();
  return video;
}

/** Restore a frame-based thumbnail even if no image is applied; return after generation and invalidate snapshots. */
export async function revertThumbnailImage(
  id: number,
  signal?: AbortSignal,
): Promise<Video> {
  const video = await request<Video>(`/api/videos/${String(id)}/thumbnail-image`, {
    method: "DELETE",
    signal,
  });
  clearListSnapshot();
  return video;
}

/** Let the browser set multipart boundaries. contentVersion must match the previewed video; stale content fails atomically with content_changed. */
export async function uploadThumbnailImage(
  id: number,
  image: Blob,
  contentVersion: string,
  signal?: AbortSignal,
): Promise<Video> {
  const form = new FormData();
  form.append("contentVersion", contentVersion);
  form.append("image", image, image instanceof File ? image.name : "image");
  const video = await request<Video>(`/api/videos/${String(id)}/thumbnail-image`, {
    method: "POST",
    body: form,
    signal,
  });
  clearListSnapshot();
  return video;
}

/** The first proximity-sorted page includes group and next-video metadata; preserve context when following nextCursor. */
export function getRelatedVideos(
  id: number,
  signal?: AbortSignal,
  cursor?: string,
  group?: number,
  smartFolder?: number,
): Promise<RelatedVideos> {
  const query = groupQuery(cursor === undefined ? {} : { cursor }, group, smartFolder);
  return request<RelatedVideos>(`/api/videos/${String(id)}/related${query}`, { signal });
}

/** Fetch members beyond the related-video window using the same group/smart-folder context. */
export function listVideoGroupMembers(
  id: number,
  offset: number,
  limit: number,
  signal?: AbortSignal,
  group?: number,
  smartFolder?: number,
): Promise<GroupMemberPage> {
  const query = groupQuery(
    { offset: String(offset), limit: String(limit) },
    group,
    smartFolder,
  );
  return request<GroupMemberPage>(`/api/videos/${String(id)}/group-members${query}`, {
    signal,
  });
}

/** The 202 response omits location and seek-preview state; reload details instead of replacing cached video data. */
export function reprobeVideo(id: number, signal?: AbortSignal): Promise<Video> {
  return request<Video>(`/api/videos/${String(id)}/probe`, { method: "POST", signal });
}

/** Representative first; unbundled videos return a single-item list. */
export function listVideoVersions(
  id: number,
  signal?: AbortSignal,
): Promise<VideoVersions> {
  return request<VideoVersions>(`/api/videos/${String(id)}/versions`, { signal });
}

/** UI-only bound: the dialog fetches each representative candidate separately; server mutation limits remain independent. */
export const maxBundleSelection = 20;

/** Invalidate snapshots because nonrepresentative versions disappear and detached lists cannot update their counts/order. */
export async function bundleVideos(
  videoIds: readonly number[],
  representativeId: number,
  signal?: AbortSignal,
): Promise<VideoVersions> {
  const versions = await request<VideoVersions>("/api/video-bundles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ videoIds, representativeId }),
    signal,
  });
  clearListSnapshot();
  return versions;
}

/** Invalidate snapshots because the displayed representative changes. */
export async function makeRepresentativeVersion(
  id: number,
  signal?: AbortSignal,
): Promise<VideoVersions> {
  const versions = await request<VideoVersions>(
    `/api/videos/${String(id)}/make-representative`,
    { method: "POST", signal },
  );
  clearListSnapshot();
  return versions;
}

/** Restore the video's pre-bundle tags, progress, and visibility; invalidate snapshots because it returns to the list. */
export async function unbundleVideo(id: number, signal?: AbortSignal): Promise<Video> {
  const video = await request<Video>(`/api/videos/${String(id)}/unbundle`, {
    method: "POST",
    signal,
  });
  clearListSnapshot();
  return video;
}

/** Return up to 200 newest candidate pairs; total counts all pairs. Refresh on scan events as ingestion changes candidates. */
export function listVersionCandidates(
  signal?: AbortSignal,
): Promise<VersionCandidatePage> {
  return request<VersionCandidatePage>("/api/version-candidates", { signal });
}

/** Persist dismissal so this pair never reappears as a candidate. */
export async function dismissVersionCandidate(
  videoIds: readonly [number, number],
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch("/api/version-candidates/dismiss", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ videoIds }),
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

/** Open the representative file on the server's machine. */
export async function openVideoFile(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/videos/${String(id)}/open`, {
    method: "POST",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

/** Return null before the first scan. */
export async function getCurrentScan(signal?: AbortSignal): Promise<Scan | null> {
  try {
    return await request<Scan>("/api/scans/current", { signal });
  } catch (error) {
    if (error instanceof RequestFailed && error.status === 404) {
      return null;
    }
    throw error;
  }
}

export const SCAN_ISSUE_PAGE_SIZE = 50;
export const MAX_SCAN_ISSUE_PAGE_SIZE = 200;

/** Return null before the first scan. */
export async function listCurrentScanIssues(
  params: { limit?: number; cursor?: string; signal?: AbortSignal } = {},
): Promise<ScanIssuePage | null> {
  const query = new URLSearchParams();
  query.set("limit", String(params.limit ?? SCAN_ISSUE_PAGE_SIZE));
  if (params.cursor !== undefined && params.cursor !== "") {
    query.set("cursor", params.cursor);
  }
  try {
    return await request<ScanIssuePage>(`/api/scans/current/issues?${query.toString()}`, {
      signal: params.signal,
    });
  } catch (error) {
    if (error instanceof RequestFailed && error.status === 404) {
      return null;
    }
    throw error;
  }
}

/** Return the current scan unchanged if one is already running. */
export function startScan(signal?: AbortSignal): Promise<Scan> {
  return request<Scan>("/api/scans", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
    signal,
  });
}

/** Rejects with scan_in_progress while a manual scan or force rescan runs; a watch scan is superseded. */
export function startRescan(
  rescan: StartRescanRequest,
  signal?: AbortSignal,
): Promise<Scan> {
  return request<Scan>("/api/scans/rescan", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(rescan),
    signal,
  });
}

export function getTranscodingSettings(
  signal?: AbortSignal,
): Promise<TranscodingSettings> {
  return request<TranscodingSettings>("/api/settings/transcoding", { signal });
}

export function updateTranscodingSettings(
  videoEncoder: VideoEncoderChoice,
  signal?: AbortSignal,
): Promise<TranscodingSettings> {
  return request<TranscodingSettings>("/api/settings/transcoding", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ videoEncoder }),
    signal,
  });
}

/** Desktop-only; other builds return 404. */
export function getNetworkSettings(signal?: AbortSignal): Promise<NetworkSettings> {
  return request<NetworkSettings>("/api/settings/network", { signal });
}

/** Return reopened network state; listener failures return 409 listen_failed. */
export function updateNetworkSettings(
  lanAccess: boolean,
  signal?: AbortSignal,
): Promise<NetworkSettings> {
  return request<NetworkSettings>("/api/settings/network", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ lanAccess }),
    signal,
  });
}

/** Empty means no proxy; available in every build. */
export function getPluginProxySettings(
  signal?: AbortSignal,
): Promise<PluginProxySettings> {
  return request<PluginProxySettings>("/api/settings/plugin-proxy", { signal });
}

/** Empty clears the proxy; invalid values return 400 invalid_request. */
export function updatePluginProxySettings(
  url: string,
  signal?: AbortSignal,
): Promise<PluginProxySettings> {
  return request<PluginProxySettings>("/api/settings/plugin-proxy", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
    signal,
  });
}

export function getAutoImportSettings(signal?: AbortSignal): Promise<AutoImportSettings> {
  return request<AutoImportSettings>("/api/settings/auto-import", { signal });
}

/** Save without starting a scan. */
export function updateAutoImportSettings(
  enabled: boolean,
  signal?: AbortSignal,
): Promise<AutoImportSettings> {
  return request<AutoImportSettings>("/api/settings/auto-import", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
    signal,
  });
}

/** Disabled before the first save. */
export function getVersionJudgingSettings(
  signal?: AbortSignal,
): Promise<VersionJudgingSettings> {
  return request<VersionJudgingSettings>("/api/settings/version-judging", { signal });
}

/** Enabling starts duplicate judging. */
export function updateVersionJudgingSettings(
  enabled: boolean,
  signal?: AbortSignal,
): Promise<VersionJudgingSettings> {
  return request<VersionJudgingSettings>("/api/settings/version-judging", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
    signal,
  });
}

export function getAutoTaggingSettings(
  signal?: AbortSignal,
): Promise<AutoTaggingSettings> {
  return request<AutoTaggingSettings>("/api/settings/auto-tagging", { signal });
}

/** Save without starting classification. */
export function updateAutoTaggingSettings(
  settings: UpdateAutoTaggingSettings,
  signal?: AbortSignal,
): Promise<AutoTaggingSettings> {
  return request<AutoTaggingSettings>("/api/settings/auto-tagging", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings),
    signal,
  });
}

/** Connection settings are independent of whether auto-tagging is enabled. */
export function getOllamaSettings(signal?: AbortSignal): Promise<OllamaSettings> {
  return request<OllamaSettings>("/api/settings/ollama", { signal });
}

/** Send only endpoint and model; the server rejects response-only fields. */
export function updateOllamaSettings(
  settings: UpdateOllamaSettings,
  signal?: AbortSignal,
): Promise<OllamaSettings> {
  const body: UpdateOllamaSettings = {
    endpoint: settings.endpoint,
    model: settings.model,
  };
  return request<OllamaSettings>("/api/settings/ollama", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
}

/** Check the saved endpoint/model without running classification. */
export function checkOllama(signal?: AbortSignal): Promise<CheckResult> {
  return request<CheckResult>("/api/settings/ollama/check", {
    method: "POST",
    signal,
  });
}

/** Return only whether an API key exists, never its value; absent provider means unconfigured. */
export function getComfyUISettings(signal?: AbortSignal): Promise<ComfyUISettings> {
  return request<ComfyUISettings>("/api/settings/comfyui", { signal });
}

/** Omitted apiKey preserves the saved key; empty clears it. */
export function updateComfyUISettings(
  settings: UpdateComfyUISettings,
  signal?: AbortSignal,
): Promise<ComfyUISettings> {
  return request<ComfyUISettings>("/api/settings/comfyui", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings),
    signal,
  });
}

/** Check the saved connection and required nodes/models; return missing dependencies in missing. */
export function checkComfyUI(signal?: AbortSignal): Promise<CheckResult> {
  return request<CheckResult>("/api/settings/comfyui/check", {
    method: "POST",
    signal,
  });
}

/** connection means configured, not successfully tested. */
export function getThumbnailEditReadiness(
  signal?: AbortSignal,
): Promise<ThumbnailEditReadiness> {
  return request<ThumbnailEditReadiness>("/api/thumbnail-edits/readiness", { signal });
}

export type ThumbnailEditStart =
  { source: "frame"; positionMs: number } | { source: "thumbnail" };

/** Starting replaces any existing session for this video; the response fixes source image and mask dimensions. */
export function startThumbnailEdit(
  videoId: number,
  start: ThumbnailEditStart,
  signal?: AbortSignal,
): Promise<ThumbnailEdit> {
  return request<ThumbnailEdit>(`/api/videos/${String(videoId)}/thumbnail-edits`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(start),
    signal,
  });
}

/** Start from an image the page drew, such as a preview's framing; the session needs no media file. */
export function startThumbnailEditFromImage(
  videoId: number,
  image: Blob,
  signal?: AbortSignal,
): Promise<ThumbnailEdit> {
  const form = new FormData();
  form.append("image", image, "framing.png");
  return request<ThumbnailEdit>(`/api/videos/${String(videoId)}/thumbnail-edits/image`, {
    method: "POST",
    body: form,
    signal,
  });
}

/** The session's starting image, a PNG at frameWidth × frameHeight. */
export async function getThumbnailEditFrame(
  editId: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const response = await apiFetch(thumbnailEditPath(editId, "/frame"), { signal });
  if (!response.ok) throw await toRequestFailed(response);
  return response.blob();
}

/** The provider's output as it came, before it is fitted to the video. */
export async function getThumbnailEditResult(
  editId: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const response = await apiFetch(thumbnailEditPath(editId, "/result"), { signal });
  if (!response.ok) throw await toRequestFailed(response);
  return response.blob();
}

function thumbnailEditPath(editId: string, suffix = ""): string {
  return `/api/thumbnail-edits/${encodeURIComponent(editId)}${suffix}`;
}

/** Reads renew the five-minute session lease; ended sessions return thumbnail_edit_not_found. */
export function getThumbnailEdit(
  editId: string,
  signal?: AbortSignal,
): Promise<ThumbnailEdit> {
  return request<ThumbnailEdit>(thumbnailEditPath(editId), { signal });
}

export interface ThumbnailEditRun {
  /** Up to 4,000 characters. */
  prompt?: string;
  /** Frame-sized PNG with opaque white paint over opaque black. */
  mask?: Blob;
  /** PNG, JPEG, or WebP up to 20 MiB. */
  reference?: Blob;
}

/** Starting cancels an active run; let the browser set multipart Content-Type and its boundary. */
export function runThumbnailEdit(
  editId: string,
  run: ThumbnailEditRun,
  signal?: AbortSignal,
): Promise<ThumbnailEdit> {
  const form = new FormData();
  if (run.prompt !== undefined) {
    form.append("prompt", run.prompt);
  }
  if (run.mask !== undefined) {
    form.append("mask", run.mask, "mask.png");
  }
  if (run.reference !== undefined) {
    form.append("reference", run.reference, "reference");
  }
  return request<ThumbnailEdit>(thumbnailEditPath(editId, "/runs"), {
    method: "POST",
    body: form,
    signal,
  });
}

/** Cancel an active run without changing the thumbnail; otherwise return current state. */
export function cancelThumbnailEdit(
  editId: string,
  signal?: AbortSignal,
): Promise<ThumbnailEdit> {
  return request<ThumbnailEdit>(thumbnailEditPath(editId, "/cancel"), {
    method: "POST",
    signal,
  });
}

/** Idempotently discard; keepalive allows submission while leaving the page. */
export async function discardThumbnailEdit(editId: string): Promise<void> {
  const response = await apiFetch(thumbnailEditPath(editId), {
    method: "DELETE",
    keepalive: true,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

/** Run one classification; loading the model may take several minutes. */
export function checkAutoTaggingClassifier(signal?: AbortSignal): Promise<CheckResult> {
  return request<CheckResult>("/api/auto-tagging/check", { method: "POST", signal });
}

/** Queue a feature's analysis run and return the queued count. */
async function queueRun(
  path: string,
  scope: "missing" | "all",
  signal?: AbortSignal,
): Promise<number> {
  const result = await request<{ queued: number }>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ scope }),
    signal,
  });
  return result.queued;
}

export function startAutoTagging(
  scope: AutoTaggingScope,
  signal?: AbortSignal,
): Promise<number> {
  return queueRun("/api/auto-tagging/runs", scope, signal);
}

export function getFaceRecognitionSettings(
  signal?: AbortSignal,
): Promise<FaceRecognitionSettings> {
  return request<FaceRecognitionSettings>("/api/settings/face-recognition", { signal });
}

/** Turning on starts the download; a changed recognizer queues every video again. */
export function updateFaceRecognitionSettings(
  settings: { enabled: boolean; recognizer: FaceRecognizer },
  signal?: AbortSignal,
): Promise<FaceRecognitionSettings> {
  return request<FaceRecognitionSettings>("/api/settings/face-recognition", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings),
    signal,
  });
}

/** Start the download again after a failure; answers with the settings body. */
export function downloadFaceRuntime(
  signal?: AbortSignal,
): Promise<FaceRecognitionSettings> {
  return request<FaceRecognitionSettings>("/api/face-recognition/download", {
    method: "POST",
    signal,
  });
}

/** Delete every person and all face data; answers what it deleted and how many videos it queued. */
export function clearPeople(
  signal?: AbortSignal,
): Promise<components["schemas"]["PeopleCleared"]> {
  return request<components["schemas"]["PeopleCleared"]>("/api/people", {
    method: "DELETE",
    signal,
  });
}

export function startFaceRecognition(
  scope: FaceRecognitionScope,
  signal?: AbortSignal,
): Promise<number> {
  return queueRun("/api/face-recognition/runs", scope, signal);
}

/** Return false if this video is already queued or running. */
export async function autoTagVideo(id: number, signal?: AbortSignal): Promise<boolean> {
  const result = await request<components["schemas"]["AutoTagVideoResult"]>(
    `/api/videos/${String(id)}/auto-tag`,
    { method: "POST", signal },
  );
  return result.queued;
}

export async function confirmVideoTag(id: number, tagId: number): Promise<void> {
  await settleVideoTag(id, tagId, "confirm");
}

/** Reject the tentative assignment and prevent auto-tagging from restoring it. */
export async function rejectVideoTag(id: number, tagId: number): Promise<void> {
  await settleVideoTag(id, tagId, "reject");
}

async function settleVideoTag(
  id: number,
  tagId: number,
  action: "confirm" | "reject",
): Promise<void> {
  const response = await apiFetch(
    `/api/videos/${String(id)}/tags/${String(tagId)}/${action}`,
    { method: "POST" },
  );
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

/** Secrets are never included in token lists. */
export async function listAPITokens(signal?: AbortSignal): Promise<APIToken[]> {
  const list = await request<components["schemas"]["APITokenList"]>("/api/api-tokens", {
    signal,
  });
  return list.items;
}

/** The secret appears only in this creation response. */
export function createAPIToken(
  name: string,
  signal?: AbortSignal,
): Promise<CreatedAPIToken> {
  return request<CreatedAPIToken>("/api/api-tokens", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  });
}

/** Revoking an absent ID succeeds. */
export async function revokeAPIToken(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/api-tokens/${String(id)}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

export function listMediaFolders(signal?: AbortSignal): Promise<MediaFolder[]> {
  return request<MediaFolder[]>("/api/media-folders", { signal });
}

/** Invalidate snapshots after root changes so returning lists cannot show removed folders or stale items. */
export async function createMediaFolder(
  path: string,
  signal?: AbortSignal,
): Promise<MediaFolder> {
  const created = await request<MediaFolder>("/api/media-folders", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
    signal,
  });
  clearListSnapshot();
  return created;
}

/** Replace one folder using its version for concurrency control. */
export async function updateMediaFolder(
  id: number,
  path: string,
  version: number,
  signal?: AbortSignal,
): Promise<MediaFolder> {
  const updated = await request<MediaFolder>(`/api/media-folders/${String(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, version }),
    signal,
  });
  clearListSnapshot();
  return updated;
}

/** Delete one folder using its version for concurrency control. */
export async function deleteMediaFolder(
  id: number,
  version: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(
    `/api/media-folders/${String(id)}?version=${String(version)}`,
    { method: "DELETE", signal },
  );
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  clearListSnapshot();
}

export function listDirectories(
  path?: string,
  signal?: AbortSignal,
): Promise<DirectoryListing> {
  const query = new URLSearchParams();
  if (path !== undefined) {
    query.set("path", path);
  }
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return request<DirectoryListing>(`/api/directories${suffix}`, { signal });
}

/** Serialize saves per video because the server applies arrival order and older concurrent requests could overwrite newer positions. */
const progressQueue = new Map<number, Promise<unknown>>();

function enqueueProgress<T>(id: number, send: () => Promise<T>): Promise<T> {
  // Send immediately when idle to avoid delaying saves during navigation.
  const previous = progressQueue.get(id);
  const next = previous === undefined ? send() : previous.then(send);
  const tail = next.catch(() => undefined);
  progressQueue.set(id, tail);
  void tail.then(() => {
    if (progressQueue.get(id) === tail) progressQueue.delete(id);
  });
  return next;
}

/** Bypass prior saves, but make later saves await both prior requests and this one. */
function sendProgressNow<T>(id: number, send: () => Promise<T>): Promise<T> {
  const previous = progressQueue.get(id) ?? Promise.resolve();
  const next = send();
  const tail = Promise.all([previous, next.catch(() => undefined)]);
  progressQueue.set(id, tail);
  void tail.then(() => {
    if (progressQueue.get(id) === tail) progressQueue.delete(id);
  });
  return next;
}

function progressBody(positionMs: number, playbackId?: string): string {
  const update: ProgressUpdate = { positionMs: Math.max(0, Math.round(positionMs)) };
  if (playbackId !== undefined) update.playbackId = playbackId;
  return JSON.stringify(update);
}

/** Serialize per video; the server derives watched state and optionally records the supplied playbackId. */
export function saveProgress(
  id: number,
  positionMs: number,
  playbackId?: string,
  signal?: AbortSignal,
): Promise<Progress> {
  const sequence = nextProgressSequence();
  const body = progressBody(positionMs, playbackId);
  return enqueueProgress(id, async () => {
    const saved = await request<Progress>(`/api/videos/${String(id)}/progress`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body,
      signal,
    });
    recordSavedProgress(id, saved, sequence);
    return saved;
  });
}

/** Use keepalive PUT on exit; page-hidden saves bypass queues. Later saves wait, but ordering against already in-flight saves cannot be guaranteed. */
export function beaconProgress(
  id: number,
  positionMs: number,
  playbackId?: string,
): void {
  const body = progressBody(positionMs, playbackId);
  const sequence = nextProgressSequence();
  const send = () =>
    apiFetch(`/api/videos/${String(id)}/progress`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body,
      keepalive: true,
    }).then(async (response) => {
      if (response.ok)
        recordSavedProgress(id, (await response.json()) as Progress, sequence);
    });
  const sending =
    document.visibilityState === "hidden"
      ? sendProgressNow(id, send)
      : enqueueProgress(id, send);
  void sending.catch(() => undefined);
}

export function streamUrl(id: number): string {
  return `/api/videos/${id}/stream`;
}

/** attempt enables lookup of the actual transcode start; quality requests downscaling. */
export function transcodeUrl(
  id: number,
  startMs = 0,
  attempt?: string,
  quality?: TranscodeQuality,
): string {
  const query = new URLSearchParams();
  if (startMs > 0) query.set("startMs", String(Math.round(startMs)));
  if (attempt !== undefined) query.set("attempt", attempt);
  if (quality !== undefined) query.set("quality", quality);
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return `/api/videos/${String(id)}/transcode.mp4${suffix}`;
}

/** Map output time zero to original milliseconds; the server waits up to six seconds before returning 404. */
export async function getTranscodeStart(
  id: number,
  attempt: string,
  signal?: AbortSignal,
): Promise<number> {
  const query = new URLSearchParams({ attempt });
  const body = await request<TranscodeStart>(
    `/api/videos/${String(id)}/transcode-start?${query.toString()}`,
    { signal },
  );
  return body.startMs;
}

/** The server rescans sidecar files on every call. */
export async function getVideoSubtitles(
  id: number,
  signal?: AbortSignal,
): Promise<SubtitleTrack[]> {
  const body = await request<SubtitleTrackList>(`/api/videos/${String(id)}/subtitles`, {
    signal,
  });
  return body.subtitles;
}

/** offsetMs maps output time zero to the original video; omit zero. */
export function subtitleUrl(id: number, file: string, offsetMs = 0): string {
  const offset = Math.max(0, Math.round(offsetMs));
  const suffix = offset > 0 ? `?offsetMs=${String(offset)}` : "";
  return `/api/videos/${String(id)}/subtitles/${encodeURIComponent(file)}${suffix}`;
}

export async function fetchSeekThumbnailSprite(
  url: string,
  signal: AbortSignal,
): Promise<SeekThumbnailSprite> {
  const response = await apiFetch(url, { signal });
  if (!response.ok) {
    throw new RequestFailed(
      response.status,
      "seek_thumbnail_failed",
      `seek thumbnail request failed: ${String(response.status)}`,
    );
  }
  return (await response.json()) as SeekThumbnailSprite;
}

export async function fetchSeekThumbnailSheet(
  url: string,
  signal: AbortSignal,
): Promise<Blob> {
  const response = await apiFetch(url, { signal });
  if (!response.ok) {
    throw new RequestFailed(
      response.status,
      "seek_thumbnail_failed",
      `seek thumbnail sheet request failed: ${String(response.status)}`,
    );
  }
  return response.blob();
}

export function isAborted(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}
