import type { Video } from "../../api/client";
import type {
  SkippedTag,
  SkippedWorkInfoValue,
  WorkInfoFetch,
  WorkInfoField,
  WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { t, type UiText } from "../../i18n";
import { typedLink } from "../../lib/typedLink";
import { creditKind, isDateRow, videoWorkInfo } from "./detailRows";

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

/** Whether the plugin fetches by link: it declares domains, and the server checks a link against them. */
export function fetchesByLink(plugin: WorkInfoPlugin): boolean {
  return plugin.domains.length > 0 && can(plugin, "fetch");
}

/** Whether the plugin reads every link, as the bundled one does, and so takes only links. */
function readsAnyLink(plugin: WorkInfoPlugin): boolean {
  return plugin.domains.includes("*");
}

/** The link a fetch sends instead of an ID: the typed text when the plugin fetches by link and the text is a link. */
export function fetchLink(plugin: WorkInfoPlugin, input: string): string | undefined {
  return fetchesByLink(plugin) ? typedLink(input) : undefined;
}

export function queryLabel(plugin: WorkInfoPlugin, by: FindBy): UiText {
  const text = t.player.workInfoFetch;
  if (runFor(plugin, by) === "search") return text.querySearch;
  if (!fetchesByLink(plugin)) return text.queryFetch;
  return readsAnyLink(plugin) ? text.queryLink : text.queryFetchOrLink;
}

/** A plugin that reads any link starts from the video's first link; others from the title or the code. */
export function initialQuery(
  plugin: WorkInfoPlugin | undefined,
  by: FindBy,
  video: Video,
): string {
  if (plugin === undefined) return "";
  if (runFor(plugin, by) === "search") return video.title;
  const info = videoWorkInfo(video);
  if (fetchesByLink(plugin) && readsAnyLink(plugin)) return info.urls[0] ?? "";
  return info.code ?? "";
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

export const TAG_FOLD = 10;
