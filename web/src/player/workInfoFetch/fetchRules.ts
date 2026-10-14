import type { Video } from "../../api/client";
import type {
  SkippedTag,
  WorkInfoFetch,
  WorkInfoField,
  WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { t, type UiText } from "../../i18n";

// 取得のダイアログの決まり（specs/049-work-info-plugins/ui-design.md「Fetch dialog」）。画面に
// 依らない判断をここに集め、ダイアログの部品は描くことだけにする。

/** RunOperation はプラグインに走らせる操作である。 */
export type RunOperation = "search" | "fetch";

/** approvedPlugins は一覧のうち承認済みのプラグインを、一覧の順のまま返す（R-11）。 */
export function approvedPlugins(plugins: readonly WorkInfoPlugin[]): WorkInfoPlugin[] {
  return plugins.filter((plugin) => plugin.state === "approved");
}

function can(plugin: WorkInfoPlugin, operation: RunOperation): boolean {
  return plugin.operations.includes(operation);
}

/** pluginLabel はプラグインの名前で、無ければファイル名である。 */
export function pluginLabel(plugin: WorkInfoPlugin): string {
  return plugin.name ?? plugin.fileName;
}

/** isWebUrl は text（前後の空白を除く）が http か https の URL として読めるかを返す。 */
export function isWebUrl(text: string): boolean {
  const trimmed = text.trim();
  if (!/^https?:\/\//i.test(trimmed)) return false;
  try {
    const url = new URL(trimmed);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

/**
 * runFor は入力 text をプラグインのどの操作で走らせるかを返す。URL は取得できるプラグインなら
 * 取得し、それ以外は検索する。検索できないプラグインに URL でない文を渡せば `notUrl` である。
 */
export function runFor(plugin: WorkInfoPlugin, text: string): RunOperation | "notUrl" {
  if (isWebUrl(text) && can(plugin, "fetch")) return "fetch";
  if (can(plugin, "search")) return "search";
  return "notUrl";
}

/** queryLabel は入力欄の名前で、プラグインの操作で決まる。 */
export function queryLabel(plugin: WorkInfoPlugin): UiText {
  const text = t.player.workInfoFetch;
  if (can(plugin, "search") && can(plugin, "fetch")) return text.queryBoth;
  return can(plugin, "search") ? text.querySearch : text.queryFetch;
}

/** initialQuery は入力欄の最初の値で、検索できるプラグインならページの題名である。 */
export function initialQuery(plugin: WorkInfoPlugin | undefined, title: string): string {
  return plugin !== undefined && can(plugin, "search") ? title : "";
}

/** canFetch はプラグインが URL から取得できるかを返す。 */
export function canFetch(plugin: WorkInfoPlugin): boolean {
  return can(plugin, "fetch");
}

/** hostOf は URL のホストで、読めなければ URL そのものである。 */
export function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/**
 * RowState は比較の行の状態である。`differs` は選べ、`missing`（結果に値が無い）と `same`
 * （今と同じ）は選べない。
 */
export type RowState = "differs" | "missing" | "same";

/** titleRow は題名の行の状態である。比べるのはページが出している題名である。 */
export function titleRow(video: Video, fetched: WorkInfoFetch): RowState {
  if (fetched.title === undefined || fetched.title === "") return "missing";
  return fetched.title === video.title ? "same" : "differs";
}

/** videoTagNames は動画に付いているタグの名前の集まりである。 */
export function videoTagNames(video: Video): Set<string> {
  return new Set(video.tags.map((tag) => tag.name));
}

/** tagsRow はタグの行の状態である。取得したどの名前も動画に付いていれば `same` である。 */
export function tagsRow(video: Video, fetched: WorkInfoFetch): RowState {
  if (fetched.tags.length === 0) return "missing";
  const names = videoTagNames(video);
  return fetched.tags.every((name) => names.has(name)) ? "same" : "differs";
}

/** rowState は項目 field の行の状態である。 */
export function rowState(
  field: WorkInfoField,
  video: Video,
  fetched: WorkInfoFetch,
): RowState {
  return field === "title" ? titleRow(video, fetched) : tagsRow(video, fetched);
}

/** FIELDS は比較の行の順である。 */
export const FIELDS: readonly WorkInfoField[] = ["title", "tags"];

/** initialSelection は比較の段を開いたときの選択で、値があって今と違う項目である。 */
export function initialSelection(video: Video, fetched: WorkInfoFetch): WorkInfoField[] {
  return FIELDS.filter((field) => rowState(field, video, fetched) === "differs");
}

/** missingSettingLabels は足りない設定のキーを、プラグインの宣言のラベルに引き直す。 */
export function missingSettingLabels(
  plugin: WorkInfoPlugin | undefined,
  keys: readonly string[],
): string[] {
  return keys.map(
    (key) => plugin?.settings.find((setting) => setting.key === key)?.label ?? key,
  );
}

/** orderSkipped は付けなかったタグを、却下したものを先に、応答の順のまま並べる。 */
export function orderSkipped(skipped: readonly SkippedTag[]): SkippedTag[] {
  return [
    ...skipped.filter((tag) => tag.reason === "rejected"),
    ...skipped.filter((tag) => tag.reason !== "rejected"),
  ];
}

/** TAG_FOLD は比較の行で、開くまでに出すタグの数である。 */
export const TAG_FOLD = 10;
