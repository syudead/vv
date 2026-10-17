import type { Video } from "../../api/client";
import type {
  SkippedTag,
  SkippedWorkInfoValue,
  WorkInfoFetch,
  WorkInfoField,
  WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { t, type UiText } from "../../i18n";
import { creditKind, detailRow, isDateRow, videoWorkInfo } from "./detailRows";

// 取得のダイアログの決まり。画面に
// 依らない判断をここに集め、ダイアログの部品は描くことだけにする。

/** RunOperation はプラグインに走らせる操作である。 */
export type RunOperation = "search" | "fetch";

/**
 * FindBy は入力欄の語の扱いで、`words` は検索の語、`id` は取得に渡す作品の ID である。search と fetch の
 * 両方を宣言するプラグインでだけ選べ、片方だけならその操作に決まる。語は URL の形でもそのまま語か ID で、
 * 画面が URL として扱うことはない（親 Issue 1232 の要件 14）。
 */
export type FindBy = "words" | "id";

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

/** canChooseFindBy はプラグインが語を検索の語にも ID にもでき、`Words`・`ID` を選べるかを返す。 */
export function canChooseFindBy(plugin: WorkInfoPlugin): boolean {
  return can(plugin, "search") && can(plugin, "fetch");
}

/**
 * runFor は入力欄の語をプラグインのどの操作で走らせるかを返す。検索できないプラグインと `ID` を選んだ
 * ときは、語を作品の ID として取得に渡し、それ以外は語として検索する。
 */
export function runFor(plugin: WorkInfoPlugin, by: FindBy): RunOperation {
  if (can(plugin, "fetch") && (by === "id" || !can(plugin, "search"))) return "fetch";
  return "search";
}

/** queryLabel は入力欄の名前で、プラグインの操作と by で決まる。 */
export function queryLabel(plugin: WorkInfoPlugin, by: FindBy): UiText {
  const text = t.player.workInfoFetch;
  return runFor(plugin, by) === "fetch" ? text.queryFetch : text.querySearch;
}

/**
 * initialQuery は入力欄の最初の値である。検索するならページの題名、ID で取得するなら動画の品番（無ければ
 * 空）である。
 */
export function initialQuery(
  plugin: WorkInfoPlugin | undefined,
  by: FindBy,
  video: Video,
): string {
  if (plugin === undefined) return "";
  return runFor(plugin, by) === "search"
    ? video.title
    : (videoWorkInfo(video).code ?? "");
}

/** canFetch はプラグインが作品の ID から取得できるかを返す。 */
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
 * RowState は比較の行の状態である。`differs` は選べ、`missing`（結果に値が無い）、`same`
 * （今と同じ）、`nothing`（足せる件が無く、結び付かない件がある一覧の行）は選べない。
 */
export type RowState = "differs" | "missing" | "same" | "nothing";

/** titleRow は題名の行の状態である。比べるのはページが出している題名である。 */
export function titleRow(video: Video, fetched: WorkInfoFetch): RowState {
  if (fetched.title === undefined || fetched.title === "") return "missing";
  return fetched.title === video.title ? "same" : "differs";
}

/** videoTagNames は動画に付いているタグの名前の集まりである。 */
export function videoTagNames(video: Video): Set<string> {
  return new Set(video.tags.map((tag) => tag.name));
}

/**
 * selectableTags は取得したタグのうち動画にまだ無い名前を、結果の順に重複なく返す。比較の段で
 * 1 つずつ選べるのはこの名前で、動画にある名前は選べない。
 */
export function selectableTags(video: Video, fetched: WorkInfoFetch): string[] {
  const names = videoTagNames(video);
  return [...new Set(fetched.tags.filter((name) => !names.has(name)))];
}

/**
 * tagsRowChecked はタグの行のチェックボックスの状態で、選べるタグが全部選ばれていれば真、
 * 一部なら `indeterminate`、1 つも無ければ偽である。
 */
export function tagsRowChecked(
  selectable: readonly string[],
  selected: readonly string[],
): boolean | "indeterminate" {
  const count = selectable.filter((name) => selected.includes(name)).length;
  if (count === 0) return false;
  return count === selectable.length ? true : "indeterminate";
}

/** withField は選択 selection に項目 field を、on なら足し、そうでなければ外して返す。 */
export function withField(
  selection: readonly WorkInfoField[],
  field: WorkInfoField,
  on: boolean,
): WorkInfoField[] {
  return on
    ? [...new Set([...selection, field])]
    : selection.filter((item) => item !== field);
}

/** tagsRow はタグの行の状態である。取得したどの名前も動画に付いていれば `same` である。 */
export function tagsRow(video: Video, fetched: WorkInfoFetch): RowState {
  if (fetched.tags.length === 0) return "missing";
  const names = videoTagNames(video);
  return fetched.tags.every((name) => names.has(name)) ? "same" : "differs";
}

/** rowState は項目 field の行の状態である。カバー画像は今のサムネイルと比べられないので、あれば `differs` である。 */
export function rowState(
  field: WorkInfoField,
  video: Video,
  fetched: WorkInfoFetch,
): RowState {
  if (field === "title") return titleRow(video, fetched);
  if (field === "tags") return tagsRow(video, fetched);
  if (field === "cover") return fetched.cover === undefined ? "missing" : "differs";
  return detailRow(field, videoWorkInfo(video), fetched).state;
}

/**
 * FIELDS は比較の行の順で、api/openapi.yaml の `WorkInfoField` の順である。
 */
export const FIELDS: readonly WorkInfoField[] = [
  "title",
  "tags",
  "cover",
  "code",
  "description",
  "dates.release",
  "dates.recorded",
  "dates.published",
  "people.performer",
  "people.director",
  "people.photographer",
  "people.streamer",
  "organizations.maker",
  "organizations.label",
  "urls",
  "externalIds",
];

/**
 * comparedFields は比較の段に出す行の項目である。題名とタグはいつも出し（値が無ければ
 * `Not in the result`）、カバー画像と作品の詳細は結果に値があるときだけ出す。
 */
export function comparedFields(video: Video, fetched: WorkInfoFetch): WorkInfoField[] {
  return FIELDS.filter(
    (field) =>
      field === "title" ||
      field === "tags" ||
      rowState(field, video, fetched) !== "missing",
  );
}

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

/** SkippedLine は結果の段の 1 行「{label}: {value} — {reason}」である。 */
export interface SkippedLine {
  key: string;
  label: UiText;
  value: string;
  reason: UiText;
}

function limitItems(field: WorkInfoField): string {
  const items: Partial<Record<WorkInfoField, string>> = t.player.workInfoFetch.limitItems;
  return items[field] ?? t.player.workInfoFetch.fields[field];
}

/** skippedReason は飛ばした値の理由の文で、項目の種類で分ける（ui-design「Words」）。 */
export function skippedReason(value: SkippedWorkInfoValue): UiText {
  const text = t.player.workInfoFetch.skippedValue;
  if (value.reason === "too_many") return text.tooMany(limitItems(value.field));
  if (value.reason === "ambiguous")
    return text.ambiguous[creditKind(value.field) ?? "person"];
  if (value.field === "cover") return text.invalidCover;
  if (isDateRow(value.field)) return text.invalidDate;
  return value.field === "urls" ? text.invalidUrl : text.invalidOther;
}

/**
 * skippedLines は結果の段の行で、付けなかったタグ（却下したものが先）、次に飛ばした値を
 * 応答の順（項目の順、項目の中は取得した順）で並べる（ui-design「Result」）。
 */
export function skippedLines(
  tags: readonly SkippedTag[],
  values: readonly SkippedWorkInfoValue[],
): SkippedLine[] {
  const text = t.player.workInfoFetch;
  return [
    ...orderSkipped(tags).map((tag) => ({
      key: `tags\u0000${tag.name}`,
      label: text.fields.tags,
      value: tag.name,
      reason: text.skipped[tag.reason],
    })),
    ...values.map((value, index) => ({
      key: `${value.field}\u0000${String(index)}`,
      label: text.fields[value.field],
      value: value.value,
      reason: skippedReason(value),
    })),
  ];
}

/** TAG_FOLD は比較の行で、開くまでに出すタグの数である。 */
export const TAG_FOLD = 10;
