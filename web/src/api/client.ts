import { reloadPage } from "../auth/pageNavigation";
import type { components, operations } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { nextProgressSequence, recordSavedProgress } from "./progressEvents";

// 型は api/openapi.yaml からの生成物を使う。契約を変えると、ここが
// コンパイルエラーになって気付ける。
export type Video = components["schemas"]["Video"];
export type SeekThumbnailSprite = components["schemas"]["SeekThumbnailSprite"];
export type VideoPage = components["schemas"]["VideoPage"];
export type VideoSort = components["schemas"]["VideoSort"];

/** videoSorts は API が受け付ける並び順のすべてである（list-api.md §3）。 */
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

/** isVideoSort は API が受け付ける並び順かどうかを返す。 */
export function isVideoSort(value: unknown): value is VideoSort {
  return typeof value === "string" && (videoSorts as readonly string[]).includes(value);
}
/**
 * TranscodeQuality はライブ変換が縮める画質である
 * （specs/027-playback-quality/contracts/transcode-quality-api.md §1）。
 */
export type TranscodeQuality = NonNullable<
  NonNullable<operations["transcodeVideo"]["parameters"]["query"]>["quality"]
>;

/** transcodeQualities は API が受け付ける画質のすべてで、大きい順に並ぶ。 */
export const transcodeQualities: readonly TranscodeQuality[] = [
  "1080p",
  "720p",
  "480p",
  "360p",
];

/** isTranscodeQuality は API が受け付ける画質かどうかを返す。 */
export function isTranscodeQuality(value: unknown): value is TranscodeQuality {
  return (
    typeof value === "string" && (transcodeQualities as readonly string[]).includes(value)
  );
}
export type WatchFilter = components["schemas"]["WatchFilter"];
export type TagRef = components["schemas"]["TagRef"];
export type LibraryGroupIds = components["schemas"]["LibraryGroupIds"];
export type VideoTag = components["schemas"]["VideoTag"];
export type VideoIdsResponse = components["schemas"]["VideoIdsResponse"];
export type FolderScope = components["schemas"]["FolderScope"];
export type VideoFolder = components["schemas"]["VideoFolder"];
export type Scan = components["schemas"]["Scan"];
export type ScanActivity = components["schemas"]["ScanActivity"];
export type ScanActivityKind = components["schemas"]["ScanActivityKind"];
export type ScanIssue = components["schemas"]["ScanIssue"];
export type ScanIssueKind = components["schemas"]["ScanIssueKind"];
export type ScanIssuePage = components["schemas"]["ScanIssuePage"];
export type Progress = components["schemas"]["Progress"];
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
export type AutoImportSettings = components["schemas"]["AutoImportSettings"];
export type FolderWatch = components["schemas"]["FolderWatch"];
export type VideoEncoderChoice = components["schemas"]["VideoEncoderChoice"];
export type VideoEncoder = components["schemas"]["VideoEncoder"];
export type EncoderAvailability = components["schemas"]["EncoderAvailability"];
export type EncoderUnavailableReason = components["schemas"]["EncoderUnavailableReason"];
export type APIToken = components["schemas"]["APIToken"];
export type CreatedAPIToken = components["schemas"]["CreatedAPIToken"];

/**
 * LibraryItem は一覧の項目1件である（api/openapi.yaml の LibraryItem）。生成した型は
 * `video` と `group` をどちらも省略可能にしているので、`kind` で読み分けられる形にする。
 */
export type LibraryItem =
  { kind: "video"; video: Video } | { kind: "group"; group: LibraryGroup };
export type VideoChanged = components["schemas"]["VideoChanged"];

// 1ページの件数。既定は契約（api/openapi.yaml）と同じ 60 で、最初の画面は
// これだけを待つ。
export const PAGE_SIZE = 60;

export type ErrorCode = ApiError["code"];
export type ErrorReason = components["schemas"]["ErrorReason"];
export type ProbeErrorCode = components["schemas"]["ProbeErrorCode"];
export type ScanErrorCode = components["schemas"]["ScanErrorCode"];

/** RequestFailedDetails は API エラーの、画面の文言に使う値である（contracts/error-api.md §1）。 */
export interface RequestFailedDetails {
  reason?: string;
  limit?: number;
  tagName?: string;
}

/**
 * RequestFailed は応答がエラーだったことを表す。画面に出す文言は i18n の errorText が
 * `reason`・`code` から作る。`message` はサーバーの英語の文（無ければ空）で、未知の
 * コードのときだけ画面に出る。未知のコードでも `status` と `code` は残す。本文が
 * JSON でなかったときの `code` は空である。
 */
export class RequestFailed extends Error {
  readonly status: number;
  readonly code: string;
  readonly reason: string | undefined;
  readonly limit: number | undefined;
  readonly tagName: string | undefined;

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
  }
}

/**
 * NetworkFailed は fetch 自体が失敗した（サーバーに届かなかった）ことを表す。
 * ブラウザの文言は画面に出さない（specs/023-english-i18n/research.md R-5）。
 */
export class NetworkFailed extends Error {
  constructor(cause: unknown) {
    super("network request failed", { cause });
    this.name = "NetworkFailed";
  }
}

/**
 * sendRequest は fetch を呼び、fetch 自体の失敗を NetworkFailed に変える。打ち切り
 * （AbortError）はそのまま投げる。`/api/*` への要求はすべてここを通す。
 */
export async function sendRequest(path: string, init?: RequestInit): Promise<Response> {
  try {
    return await fetch(path, init);
  } catch (error) {
    if (isAborted(error)) throw error;
    throw new NetworkFailed(error);
  }
}

// --- 見る人が変わったときの読み直し ---

/** renderedAudience はゲートが確かめた、今描いている相手である。確かめる前は null。 */
let renderedAudience: "owner" | "guest" | null = null;
/** reloading は読み直しを1度始めたかどうか。2度目からは何もしない。 */
let reloading = false;

/**
 * setRenderedAudience は、ゲートが確かめた相手を覚える。所有者として描いている間に
 * 見る人が変わったと分かったら、apiFetch がページを読み直す。
 */
export function setRenderedAudience(audience: "owner" | "guest" | null): void {
  renderedAudience = audience;
  reloading = false;
}

/**
 * reloadForViewerChange は、見る人が変わったときに今の URL をページごと1度だけ
 * 読み直す。何度呼んでも読み直しは1回である。トーストは出さない。読み直した画面は
 * ゲートで状態を取り直す。
 */
export function reloadForViewerChange(): void {
  if (reloading) return;
  reloading = true;
  reloadPage();
}

/**
 * reloadIfNoLongerOwner は、所有者として描いている間に確かめた見る人の状態が所有者で
 * なくなっていたら、ページを1度だけ読み直して true を返す。応答の本文や状態を
 * 読めない経路（`/api/events` の EventSource、`video` 要素）が、状態の確認
 * （GET /api/auth/session）の答えを渡すために使う。
 */
export function reloadIfNoLongerOwner(state: string): boolean {
  if (renderedAudience !== "owner" || state === "owner") return false;
  reloadForViewerChange();
  return true;
}

/**
 * viewerChanged は、所有者として描いている間に、この応答が「もう所有者ではない」
 * ことを示すか（401 か X-VV-Audience: guest）を返す。
 */
function viewerChanged(response: Response): boolean {
  if (renderedAudience !== "owner") return false;
  return response.status === 401 || response.headers.get("X-VV-Audience") === "guest";
}

/**
 * apiFetch は `/api/*` への fetch である。所有者として描いている間に 401 か
 * `X-VV-Audience: guest` を受けたら、ページを1度だけ読み直し、応答は返さない
 * （決して解決しない）。ゲストとして処理した応答を、所有者の画面に一瞬でも
 * 描かないためである。認証の経路（auth.ts）はこれを通さない。
 */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const response = await sendRequest(path, init);
  if (viewerChanged(response)) {
    reloadForViewerChange();
    return new Promise<Response>(() => undefined);
  }
  return response;
}

/**
 * request は JSON を取りに行く薄いラッパである。
 *
 * データ取得ライブラリは入れない。この時点の要求は「一覧（無限スクロール）・
 * 詳細・進捗送信」の3種類しかなく、キャッシュ無効化の関係も単純だからである。
 * 画面をまたぐキャッシュ整合や楽観更新が要る段階で再検討する。
 */
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await apiFetch(path, init);

  if (!response.ok) {
    throw await toRequestFailed(response);
  }
  return (await response.json()) as T;
}

/** toRequestFailed は誤りの応答を、画面が文言を選べる形へ変える。 */
export async function toRequestFailed(response: Response): Promise<RequestFailed> {
  let body: Partial<ApiError> | null = null;
  try {
    body = (await response.json()) as Partial<ApiError> | null;
  } catch {
    // JSON で返ってこない場合もある（経路の取り違え、中継の失敗）。
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
    },
  );
}

/** MAX_QUERY_LENGTH は検索語に許す長さである（api/openapi.yaml の maxLength）。 */
export const MAX_QUERY_LENGTH = 100;

/**
 * ListFilterParams は2つの一覧（ライブラリとフォルダ）が共通に受ける条件である
 * （specs/013-library-search/contracts/list-api.md §2・§3）。
 */
export interface ListFilterParams {
  /** 検索語。空なら送らない。100 文字を越える分は切り詰める。 */
  query?: string;
  /** 視聴状態の絞り込み。省略時はサーバーの既定（all）。 */
  watch?: WatchFilter;
  /** true ならブラウザで再生できる動画だけにする。false は既定なので送らない。 */
  playable?: boolean;
  /**
   * true ならお気に入りの項目だけにする。false は既定なので送らない
   * （specs/035-favorites/contracts/screen-api.md §2）。
   */
  favorite?: boolean;
  sort?: VideoSort;
  /** sort=random の並びを決める値（0 以上 2147483647 以下）。 */
  seed?: number;
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

/** MAX_TAG_FILTER_COUNT はタグでの絞り込みに使える id の最大個数である。 */
export const MAX_TAG_FILTER_COUNT = 16;

/**
 * ListVideosParams は一覧の問い合わせ条件である。`tag` は listFolderVideos には
 * 無い（フォルダ画面のタグ絞り込みは対象外。
 * specs/014-video-tags/contracts/tags-api.md §5）。
 */
export interface ListVideosParams extends ListFilterParams {
  /** 絞り込むタグの id（すべて持つ動画だけにする AND）。最大16個。書く順は問わない。 */
  tag?: number[];
}

/** setListFilters は共通の条件を問い合わせに載せる。 */
function setListFilters(query: URLSearchParams, params: ListFilterParams): void {
  if (params.query !== undefined && params.query !== "") {
    // サーバーと同じく符号位置で数えて切る（サロゲートペアを割らない）。
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

/** setTagFilter はタグの絞り込みを問い合わせに載せる（listVideos・listLibrary・listLibraryIds）。 */
function setTagFilter(query: URLSearchParams, tag?: number[]): void {
  if (tag === undefined) return;
  for (const id of tag.slice(0, MAX_TAG_FILTER_COUNT)) {
    query.append("tag", String(id));
  }
}

/** listVideos は一覧を1ページ取得する。 */
export function listVideos(params: ListVideosParams = {}): Promise<VideoPage> {
  const query = new URLSearchParams();
  setListFilters(query, params);
  setTagFilter(query, params.tag);
  return request<VideoPage>(`/api/videos?${query.toString()}`, { signal: params.signal });
}

/**
 * LibraryPageResponse はライブラリの一覧1ページである（GET /api/library）。項目は
 * `kind` で読み分けられる LibraryItem にそろえてある。
 */
export type LibraryPageResponse = Omit<components["schemas"]["LibraryPage"], "items"> & {
  items: LibraryItem[];
};

/**
 * toLibraryItem は生成した型の項目を、`kind` で読み分けられる形にする。契約に反して
 * 中身の欠けた項目は捨てる（null）。
 */
function toLibraryItem(item: components["schemas"]["LibraryItem"]): LibraryItem | null {
  if (item.kind === "video" && item.video !== undefined) {
    return { kind: "video", video: item.video };
  }
  if (item.kind === "group" && item.group !== undefined) {
    return { kind: "group", group: item.group };
  }
  return null;
}

/**
 * listLibrary はライブラリの一覧を1ページ取得する。項目は動画か、フォルダの
 * グループ1件である（specs/017-folder-groups/contracts/library-api.md §1）。
 */
export async function listLibrary(
  params: ListVideosParams = {},
): Promise<LibraryPageResponse> {
  const query = new URLSearchParams();
  setListFilters(query, params);
  setTagFilter(query, params.tag);
  const page = await request<components["schemas"]["LibraryPage"]>(
    `/api/library?${query.toString()}`,
    { signal: params.signal },
  );
  return {
    ...page,
    items: page.items.flatMap((item) => {
      const converted = toLibraryItem(item);
      return converted === null ? [] : [converted];
    }),
  };
}

/** ListLibraryIdsParams は「すべて選択」用の全件 id の問い合わせ条件である。 */
export interface ListLibraryIdsParams {
  query?: string;
  watch?: WatchFilter;
  playable?: boolean;
  /** true ならお気に入りの項目だけにする。false は既定なので送らない。 */
  favorite?: boolean;
  tag?: number[];
  signal?: AbortSignal;
}

/**
 * listLibraryIds は listLibrary と同じ条件に合う項目の動画の id（グループは全メンバー）を、
 * ページングせずに取得する（「すべて選択」用。
 * specs/017-folder-groups/contracts/library-api.md §2）。グループの項目は `groups` にも
 * フォルダと全メンバーの id として入る（specs/035-favorites/contracts/screen-api.md §3）。
 */
export function listLibraryIds(
  params: ListLibraryIdsParams = {},
): Promise<VideoIdsResponse> {
  const query = new URLSearchParams();
  if (params.query !== undefined && params.query !== "") {
    query.set("query", Array.from(params.query).slice(0, MAX_QUERY_LENGTH).join(""));
  }
  if (params.watch !== undefined) query.set("watch", params.watch);
  if (params.playable === true) query.set("playable", "true");
  if (params.favorite === true) query.set("favorite", "true");
  setTagFilter(query, params.tag);
  return request<VideoIdsResponse>(`/api/library/ids?${query.toString()}`, {
    signal: params.signal,
  });
}

/** FolderRef はフォルダを指す。登録フォルダの id と `/` 区切りの相対パスの組である。 */
export interface FolderRef {
  rootId: number;
  /** 登録フォルダ自身は空文字。 */
  path: string;
}

/** listRootFolders はフォルダ画面の最上位（登録済みメディアフォルダ）を取得する。 */
export function listRootFolders(signal?: AbortSignal): Promise<RootFolderListing> {
  return request<RootFolderListing>("/api/folders", { signal });
}

/** folderQuery は相対パスを問い合わせに載せる。登録フォルダ自身では省く。 */
function folderQuery(folder: FolderRef): URLSearchParams {
  const query = new URLSearchParams();
  if (folder.path !== "") query.set("path", folder.path);
  return query;
}

/** getFolder はフォルダ1件と直下の子フォルダを取得する。 */
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

/** ListFolderVideosParams はフォルダの動画の問い合わせ条件である。 */
export interface ListFolderVideosParams extends ListFilterParams {
  folder: FolderRef;
  /** direct（既定）は直下だけ、subtree は配下すべて。省略時は送らない。 */
  scope?: FolderScope;
}

/** listFolderVideos はフォルダの動画を1ページ取得する。 */
export function listFolderVideos(params: ListFolderVideosParams): Promise<VideoPage> {
  const query = folderQuery(params.folder);
  if (params.scope !== undefined) query.set("scope", params.scope);
  setListFilters(query, params);
  return request<VideoPage>(
    `/api/folders/${String(params.folder.rootId)}/videos?${query.toString()}`,
    { signal: params.signal },
  );
}

/**
 * getFolderGroup はフォルダのグループ1件を、絞り込みに関係なく全メンバーから取得する
 * （一覧に残っているグループの項目の取り直し。specs/017-folder-groups/contracts/library-api.md §3）。
 * そのフォルダが今グループでないか無いときは 404 の RequestFailed で失敗する。
 */
export function getFolderGroup(
  folder: FolderRef,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  const query = folderQuery(folder);
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return request<LibraryGroup>(`/api/folders/${String(folder.rootId)}/group${suffix}`, {
    signal,
  });
}

/** getVideo は動画1件の詳細を取得する。 */
export function getVideo(id: number, signal?: AbortSignal): Promise<Video> {
  return request<Video>(`/api/videos/${id}`, { signal });
}

/**
 * setVideoDisplayName は動画の表示名を設定する。前後の空白を除いて空なら解除し、元の題名に戻す。
 * 応答は `getVideo` と同じ形の動画で、`title`・`fileTitle`・`displayName` が反映済みである
 * （specs/029-video-overrides/contracts/screen-api.md §1）。
 *
 * 成功したら一覧の控えを捨てる。控えは離れた一覧の題名・並び・検索の当たりを持ち、
 * 一覧が外れている間の `video` 通知では直らないので、戻ったときは読み直す。
 */
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

/**
 * setVideoThumbnailPosition は動画の代表サムネイルを `positionMs`（ミリ秒）の場面で作り直す。
 * `null` は位置を解除し、自動の位置で作り直す。応答は生成が終わってから返り、`getVideo` と
 * 同じ形の動画で、新しい版の `thumbnailUrl` と `thumbnailPositionMs` が反映済みである
 * （specs/029-video-overrides/contracts/screen-api.md §2）。
 *
 * 成功したら一覧の控えを捨てる。控えは離れた一覧のカードの `thumbnailUrl` を持ち、
 * 一覧が外れている間の `video` 通知では直らないので、戻ったときは読み直す。
 */
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

/** getRelatedVideos は動画詳細画面の関連動画（最大 20 件と次の動画の id）を取得する。 */
export function getRelatedVideos(
  id: number,
  signal?: AbortSignal,
): Promise<RelatedVideos> {
  return request<RelatedVideos>(`/api/videos/${String(id)}/related`, { signal });
}

/**
 * listVideoGroupMembers は動画が属するグループのメンバーを、グループの中の並びで
 * offset 本目（0 始まり）から最大 limit 本取得する。関連動画の group の窓の外を読む。
 */
export function listVideoGroupMembers(
  id: number,
  offset: number,
  limit: number,
  signal?: AbortSignal,
): Promise<GroupMemberPage> {
  const query = new URLSearchParams({ offset: String(offset), limit: String(limit) });
  return request<GroupMemberPage>(
    `/api/videos/${String(id)}/group-members?${query.toString()}`,
    { signal },
  );
}

/**
 * reprobeVideo は読み取りに失敗した動画を、もう一度読み取りの処理へ入れる。
 *
 * 202 の本文は所在とシーク用プレビューの状態を持たない（動画 1 件の取得にだけ入る）。
 * 画面はこの本文で控えを置き換えず、取り直して使う。
 */
export function reprobeVideo(id: number, signal?: AbortSignal): Promise<Video> {
  return request<Video>(`/api/videos/${String(id)}/probe`, { method: "POST", signal });
}

/**
 * listVideoVersions は動画の集まり（同じ動画の別バージョン）の全バージョンを、代表を先頭に取得する。
 * 集まりに属さなければ `items` はその 1 本である
 * （specs/030-video-versions/contracts/screen-api.md §1）。
 */
export function listVideoVersions(
  id: number,
  signal?: AbortSignal,
): Promise<VideoVersions> {
  return request<VideoVersions>(`/api/videos/${String(id)}/versions`, { signal });
}

/**
 * maxBundleSelection は画面から一度に束ねる動画の上限である。束ねる窓は選んだ 1 本ごとに
 * `GET /api/videos/{id}` を送り、代表の候補の行を並べるので、画面の側だけで絞る。サーバーの
 * `too_many_videos` の上限（`POST /api/video-tags` と同じ）は変えない
 * （specs/030-video-versions/ui-design.md「Bundle action」）。
 */
export const maxBundleSelection = 20;

/**
 * bundleVideos は `videoIds` の動画を 1 つの集まりに束ね、`representativeId` を一覧に出す代表にする
 * （specs/030-video-versions/contracts/screen-api.md §2）。応答は新しい集まりの全バージョンである。
 *
 * 成功したら一覧の控えを捨てる。代表以外のバージョンが一覧から消え、一覧が外れている間の
 * `video` 通知では控えの項目の数と並びが直らないので、戻ったときは読み直す。
 */
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

/**
 * makeRepresentativeVersion は動画をその集まりの代表（一覧に出す 1 件）にする。応答は集まりの
 * 全バージョンである（specs/030-video-versions/contracts/screen-api.md §3）。
 *
 * 成功したら一覧の控えを捨てる（一覧の 1 件が別の動画に替わる）。
 */
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

/**
 * unbundleVideo は動画をその集まりから外す。応答は `getVideo` と同じ形の外した動画で、タグ・
 * 再生位置・公開の設定は束ねる前の値である（specs/030-video-versions/contracts/screen-api.md §4）。
 *
 * 成功したら一覧の控えを捨てる（外した動画が一覧に戻る）。
 */
export async function unbundleVideo(id: number, signal?: AbortSignal): Promise<Video> {
  const video = await request<Video>(`/api/videos/${String(id)}/unbundle`, {
    method: "POST",
    signal,
  });
  clearListSnapshot();
  return video;
}

/**
 * listVersionCandidates は「同じ動画かもしれない」候補の組を新しい順に取得する（最大 200 組、`total` は
 * 全件）。各組の `videos` は id の小さい順で、`getVideo` と同じ形である
 * （specs/030-video-versions/contracts/screen-api.md §5）。候補は取り込みで増減するので、`scan` の
 * 通知で取り直す（§6）。
 */
export function listVersionCandidates(
  signal?: AbortSignal,
): Promise<VersionCandidatePage> {
  return request<VersionCandidatePage>("/api/version-candidates", { signal });
}

/**
 * dismissVersionCandidate は 2 本の動画を「違う動画」と記録し、その組を候補から外す。記録した組は
 * 以後候補に出ない（specs/030-video-versions/contracts/screen-api.md §5）。「同じ動画」は
 * `bundleVideos` に 2 本と代表を渡す。
 */
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

/** openVideoFile はサーバーの PC で、動画の代表の所在を既定のアプリで開く。 */
export async function openVideoFile(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/videos/${String(id)}/open`, {
    method: "POST",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

/** getCurrentScan は直近の取り込みの状態を取得する。一度も取り込んでいなければ null。 */
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

/** SCAN_ISSUE_PAGE_SIZE は問題の一覧の1ページの件数である（契約の既定と同じ 50）。 */
export const SCAN_ISSUE_PAGE_SIZE = 50;
/** MAX_SCAN_ISSUE_PAGE_SIZE は問題の一覧の1ページに求められる件数の上限である。 */
export const MAX_SCAN_ISSUE_PAGE_SIZE = 200;

/**
 * listCurrentScanIssues は直近の取り込みの問題を1ページ取得する
 * （specs/024-import-progress/contracts/scan-api.md §3）。一度も取り込んでいなければ null。
 */
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

/** startScan は取り込みを促す。実行中なら、実行中のものがそのまま返る。 */
export function startScan(signal?: AbortSignal): Promise<Scan> {
  return request<Scan>("/api/scans", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
    signal,
  });
}

/** getTranscodingSettings はライブ変換の映像エンコード方式と、各方式の確認結果を取得する。 */
export function getTranscodingSettings(
  signal?: AbortSignal,
): Promise<TranscodingSettings> {
  return request<TranscodingSettings>("/api/settings/transcoding", { signal });
}

/** updateTranscodingSettings は方式を保存し、保存後の状態全体を返す。 */
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

/**
 * getNetworkSettings は LAN からの接続の許可と、許可中に開くアドレスを取得する。デスクトップ版で
 * なければ `404` の RequestFailed になる（specs/037-windows-app/contracts/network-settings-api.md §2）。
 */
export function getNetworkSettings(signal?: AbortSignal): Promise<NetworkSettings> {
  return request<NetworkSettings>("/api/settings/network", { signal });
}

/**
 * updateNetworkSettings は LAN からの接続の許可を切り替え、開き直したあとの状態を返す。
 * 開き直せなければ `409`・reason `listen_failed` になる（同 §3）。
 */
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

/**
 * getAutoImportSettings は自動の取り込みの選択と、監視が今していることを取得する
 * （specs/042-folder-watch-import/contracts/screen-api.md）。
 */
export function getAutoImportSettings(signal?: AbortSignal): Promise<AutoImportSettings> {
  return request<AutoImportSettings>("/api/settings/auto-import", { signal });
}

/** updateAutoImportSettings は選択を保存し、保存後の状態を返す。取り込みは始めない。 */
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

/** listAPITokens は発行済みの API トークンを作成日時の新しい順で取得する。平文は含まない。 */
export async function listAPITokens(signal?: AbortSignal): Promise<APIToken[]> {
  const list = await request<components["schemas"]["APITokenList"]>("/api/api-tokens", {
    signal,
  });
  return list.items;
}

/** createAPIToken は API トークンを発行する。平文（`secret`）はこの応答にだけ現れる。 */
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

/** revokeAPIToken は API トークンを失効する。無い id も成功として扱う（contracts/token-api.md）。 */
export async function revokeAPIToken(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/api-tokens/${String(id)}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

/** listMediaFolders は登録rootをid順で取得する。 */
export function listMediaFolders(signal?: AbortSignal): Promise<MediaFolder[]> {
  return request<MediaFolder[]>("/api/media-folders", { signal });
}

/**
 * createMediaFolder は選択済みpathを1件追加する。
 *
 * 登録フォルダを変える3つの操作は、成功したら一覧の控えを捨てる。控えは
 * 登録の変更を知らないので、戻ったときに消えたフォルダや古い一覧を出してしまう。
 */
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

/** updateMediaFolder は1件だけをversion付きで置き換える。 */
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

/** deleteMediaFolder は1件だけをversion付きで削除する。 */
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

/** listDirectories はpickerのnavigation起点または指定path直下を取得する。 */
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

/**
 * progressQueue は動画ごとに、送った保存の完了を待つ連なりである。
 *
 * サーバーは届いた保存をそのまま上書きするので、同じ動画の保存を並行して送ると、
 * 先に送った古い位置が後から届いて残ることがある。1件ずつ順に送る。
 */
const progressQueue = new Map<number, Promise<unknown>>();

function enqueueProgress<T>(id: number, send: () => Promise<T>): Promise<T> {
  // 送信中の保存が無ければ、その場で送る（画面を離れる瞬間の送信を遅らせない）。
  const previous = progressQueue.get(id);
  const next = previous === undefined ? send() : previous.then(send);
  const tail = next.catch(() => undefined);
  progressQueue.set(id, tail);
  void tail.then(() => {
    if (progressQueue.get(id) === tail) progressQueue.delete(id);
  });
  return next;
}

/**
 * sendProgressNow は順番を待たずにすぐ送り、それでも以後の保存は、それまでの
 * 保存とこの送信の両方が終わってから送るようにする。
 */
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

/**
 * saveProgress は再生位置を送る。視聴済みの判定はサーバー側が行うので、
 * ここでは位置だけを送る。同じ動画の保存は、前の保存が終わってから送る。
 */
export function saveProgress(
  id: number,
  positionMs: number,
  signal?: AbortSignal,
): Promise<Progress> {
  const sequence = nextProgressSequence();
  return enqueueProgress(id, async () => {
    const saved = await request<Progress>(`/api/videos/${String(id)}/progress`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ positionMs: Math.max(0, Math.round(positionMs)) }),
      signal,
    });
    recordSavedProgress(id, saved, sequence);
    return saved;
  });
}

/**
 * beaconProgress は離脱時に再生位置を送る。
 *
 * keepalive付きfetchで、既存のPUT契約を保ったまま画面離脱後も送信を継続する。
 * アプリの中で画面を移るときは、送信中の保存が終わってから送る。ページ自体が
 * 隠れる（タブを閉じるなど）ときは待てないので、すぐに送る。その場合も、以後の
 * 保存はこの送信の完了を待つ。すでに送信中だった保存との順序は、クライアントだけ
 * では保証できない（サーバーは届いた順に上書きする）。
 */
export function beaconProgress(id: number, positionMs: number): void {
  const body = JSON.stringify({ positionMs: Math.max(0, Math.round(positionMs)) });
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

/** streamUrl は動画本体の取得先を返す。 */
export function streamUrl(id: number): string {
  return `/api/videos/${id}/stream`;
}

/**
 * transcodeUrl は指定した元動画時刻からライブ変換する取得先を返す。attempt を渡すと、
 * 実際の開始位置を getTranscodeStart で引けるよう URL に付ける
 * （specs/018-live-transcode-seek/contracts/transcode-start-api.md §1）。quality を渡すと
 * その画質に縮めて変換する（specs/027-playback-quality/contracts/transcode-quality-api.md §1）。
 */
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

/**
 * getTranscodeStart は、同じ attempt を付けたライブ変換の出力の時間軸の 0 が元動画の
 * どの時刻か（ミリ秒）を取得する。サーバーは決まるまで最大 6 秒待ち、決まらなければ
 * 404 を返す（contracts/transcode-start-api.md §2）。
 */
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

/**
 * getVideoSubtitles は動画の隣に置いた字幕ファイルの一覧を取得する。サーバーは呼ぶたびに
 * フォルダを読み直す（specs/028-sidecar-subtitles/contracts/subtitles-api.md §1）。
 */
export async function getVideoSubtitles(
  id: number,
  signal?: AbortSignal,
): Promise<SubtitleTrack[]> {
  const body = await request<SubtitleTrackList>(`/api/videos/${String(id)}/subtitles`, {
    signal,
  });
  return body.subtitles;
}

/**
 * subtitleUrl は字幕ファイル file を WebVTT で取得する先を返す。offsetMs は再生の時間軸の
 * 0 に当たる元動画の時刻（ミリ秒）で、0 なら付けない（contracts/subtitles-api.md §2）。
 */
export function subtitleUrl(id: number, file: string, offsetMs = 0): string {
  const offset = Math.max(0, Math.round(offsetMs));
  const suffix = offset > 0 ? `?offsetMs=${String(offset)}` : "";
  return `/api/videos/${String(id)}/subtitles/${encodeURIComponent(file)}${suffix}`;
}

/**
 * fetchSeekThumbnailSprite はシークプレビューのスプライトの配置情報を取得する。url は
 * Video.seekThumbnailUrl（specs/021-seek-thumbnail-sprite/contracts/seek-sprite-api.md §2）。
 */
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

/**
 * fetchSeekThumbnailSheet はスプライトのシート（配置情報の sheets の URL）の JPEG を
 * 取得する（contracts/seek-sprite-api.md §3）。
 */
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

/** isAborted は「利用者が先に進んだので打ち切った」だけかどうかを返す。 */
export function isAborted(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}
