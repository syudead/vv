import type { Video } from "../../api/client";
import type {
  SkippedTag,
  SkippedWorkInfoValue,
  WorkInfoFetch,
  WorkInfoField,
  WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { t, type UiText } from "../../i18n";
import { skippedPersonReason } from "../../credits/person/pluginFetch/personCompareRows";
import { creditKind, detailRow, isDateRow, videoWorkInfo } from "./detailRows";
import { skippedLookupName } from "./lookupRules";

export type RunOperation = "search" | "fetch";

/** Treat input as search words or an opaque work ID according to plugin capabilities, even when it looks like a URL. */
export type FindBy = "words" | "id";

function approvedPlugins(
  plugins: readonly WorkInfoPlugin[],
  kind: WorkInfoPlugin["kind"],
): WorkInfoPlugin[] {
  return plugins.filter((plugin) => plugin.kind === kind && plugin.state === "approved");
}

/** Person plugins share the list but never fetch work information. */
export function approvedWorkPlugins(
  plugins: readonly WorkInfoPlugin[],
): WorkInfoPlugin[] {
  return approvedPlugins(plugins, "work");
}

/** The plugins a person page fetches from. */
export function approvedPersonPlugins(
  plugins: readonly WorkInfoPlugin[],
): WorkInfoPlugin[] {
  return approvedPlugins(plugins, "person");
}

function can(plugin: WorkInfoPlugin, operation: RunOperation): boolean {
  return plugin.operations.includes(operation);
}

export function pluginLabel(plugin: WorkInfoPlugin): string {
  return plugin.name ?? plugin.fileName;
}

export function canChooseFindBy(plugin: WorkInfoPlugin): boolean {
  return can(plugin, "search") && can(plugin, "fetch");
}

export function runFor(plugin: WorkInfoPlugin, by: FindBy): RunOperation {
  if (can(plugin, "fetch") && (by === "id" || !can(plugin, "search"))) return "fetch";
  return "search";
}

export function queryLabel(plugin: WorkInfoPlugin, by: FindBy): UiText {
  const text = t.player.workInfoFetch;
  return runFor(plugin, by) === "fetch" ? text.queryFetch : text.querySearch;
}

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

export function canFetch(plugin: WorkInfoPlugin): boolean {
  return can(plugin, "fetch");
}

export function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/** Only differs is selectable; nothing means unresolved entries with no additions. */
export type RowState = "differs" | "missing" | "same" | "nothing";

export function titleRow(video: Video, fetched: WorkInfoFetch): RowState {
  if (fetched.title === undefined || fetched.title === "") return "missing";
  return fetched.title === video.title ? "same" : "differs";
}

export function videoTagNames(video: Video): Set<string> {
  return new Set(video.tags.map((tag) => tag.name));
}

export function selectableTags(video: Video, fetched: WorkInfoFetch): string[] {
  const names = videoTagNames(video);
  return [...new Set(fetched.tags.filter((name) => !names.has(name)))];
}

export function tagsRowChecked(
  selectable: readonly string[],
  selected: readonly string[],
): boolean | "indeterminate" {
  const count = selectable.filter((name) => selected.includes(name)).length;
  if (count === 0) return false;
  return count === selectable.length ? true : "indeterminate";
}

export function withField(
  selection: readonly WorkInfoField[],
  field: WorkInfoField,
  on: boolean,
): WorkInfoField[] {
  return on
    ? [...new Set([...selection, field])]
    : selection.filter((item) => item !== field);
}

export function tagsRow(video: Video, fetched: WorkInfoFetch): RowState {
  if (fetched.tags.length === 0) return "missing";
  const names = videoTagNames(video);
  return fetched.tags.every((name) => names.has(name)) ? "same" : "differs";
}

/** Cover images cannot be compared with current thumbnails; any fetched cover differs. */
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

/** Match WorkInfoField order in api/openapi.yaml. */
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

/** Always show title and tags; show cover and detail rows only when the result has values. */
export function comparedFields(video: Video, fetched: WorkInfoFetch): WorkInfoField[] {
  return FIELDS.filter(
    (field) =>
      field === "title" ||
      field === "tags" ||
      rowState(field, video, fetched) !== "missing",
  );
}

export function initialSelection(video: Video, fetched: WorkInfoFetch): WorkInfoField[] {
  return FIELDS.filter((field) => rowState(field, video, fetched) === "differs");
}

export function missingSettingLabels(
  plugin: WorkInfoPlugin | undefined,
  keys: readonly string[],
): string[] {
  return keys.map(
    (key) => plugin?.settings.find((setting) => setting.key === key)?.label ?? key,
  );
}

/** Place rejected tags first while retaining response order within each group. */
export function orderSkipped(skipped: readonly SkippedTag[]): SkippedTag[] {
  return [
    ...skipped.filter((tag) => tag.reason === "rejected"),
    ...skipped.filter((tag) => tag.reason !== "rejected"),
  ];
}

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

export function skippedReason(value: SkippedWorkInfoValue): UiText {
  const text = t.player.workInfoFetch.skippedValue;
  if (value.reason === "too_many") return text.tooMany(limitItems(value.field));
  if (value.reason === "ambiguous")
    return text.ambiguous[creditKind(value.field) ?? "person"];
  if (value.field === "cover") return text.invalidCover;
  if (isDateRow(value.field)) return text.invalidDate;
  return value.field === "urls" ? text.invalidUrl : text.invalidOther;
}

function lookupReason(value: SkippedWorkInfoValue): UiText {
  const text = t.credits.personPage.pluginFetch.skipped;
  if (value.reason === "stale") return text.stale;
  if (value.reason === "person_changed") return text.personChanged;
  if (value.item === undefined) return text.invalid;
  return skippedPersonReason({
    item: value.item,
    reason: value.reason === "ambiguous" ? "invalid" : value.reason,
    holderName: value.holderName,
  });
}

/** A lookup's item value is labelled with its person; a whole lookup left out keeps the role, its value naming the person. */
function skippedLabel(value: SkippedWorkInfoValue, result?: WorkInfoFetch): UiText {
  const text = t.player.workInfoFetch;
  const name = result === undefined ? undefined : skippedLookupName(result, value);
  if (value.item === undefined || name === undefined) return text.fields[value.field];
  return text.lookupItem(name, t.credits.personPage.pluginFetch.items[value.item]);
}

export function skippedLines(
  tags: readonly SkippedTag[],
  values: readonly SkippedWorkInfoValue[],
  result?: WorkInfoFetch,
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
      label: skippedLabel(value, result),
      value: value.value,
      reason: value.lookup === undefined ? skippedReason(value) : lookupReason(value),
    })),
  ];
}

export const TAG_FOLD = 10;
