import { useMemo } from "react";

import type { AddedLengthFilterParams } from "../api/client";
import { formatNumber, t, type UiText } from "../i18n";

/**
 * addedLengthCriteria は、ライブラリの絞り込みの追加日と長さ（URL の `addedWithin`・
 * `minDuration`・`maxDuration`）の読み書きと、ポップオーバーとスマートフォルダの窓が出す
 * 選択肢を受け持つ（specs/052-smart-folders/ui-design.md「Added and length」、research.md R-4）。
 * タグや人物と同じく共有の `ListCriteria` には足さない — 足すとフォルダ画面も読んでしまう。
 * タグや人物と違ってゲストも使える（所有者のデータに依らない）。
 */

/** URL の `addedWithin` パラメータの名前。 */
export const ADDED_WITHIN_PARAM = "addedWithin";
/** URL の `minDuration` パラメータの名前。 */
export const MIN_DURATION_PARAM = "minDuration";
/** URL の `maxDuration` パラメータの名前。 */
export const MAX_DURATION_PARAM = "maxDuration";

/** addedLengthParams は 3 つのパラメータの名前である（ゲストも残す画面固有のパラメータ）。 */
export const addedLengthParams: readonly string[] = [
  ADDED_WITHIN_PARAM,
  MIN_DURATION_PARAM,
  MAX_DURATION_PARAM,
];

/** 追加日の日数の範囲（contracts/screen-api.md の SmartRule と一覧のパラメータ）。 */
export const MIN_ADDED_WITHIN = 1;
export const MAX_ADDED_WITHIN = 36500;
/** 長さの秒数の上限（下限は 0）。 */
export const MAX_DURATION_SECONDS = 360000;

/** AddedLengthFilter は追加日と長さの絞り込みである。どれも省けば絞らない。 */
export type AddedLengthFilter = AddedLengthFilterParams;

/** LengthRange は長さの下限と上限（秒、含む）である。 */
export interface LengthRange {
  minDuration?: number;
  maxDuration?: number;
}

/** addedPresets は `Added` の選択肢の日数である（`Any time` を除く）。 */
const addedPresets: readonly number[] = [1, 7, 30, 90, 365];

type LengthPresetKey = "under5" | "from5to20" | "from20to60" | "overHour";

/** lengthPresets は `Length` の選択肢である（`Any length` を除く）。 */
const lengthPresets: readonly { key: LengthPresetKey; range: LengthRange }[] = [
  { key: "under5", range: { maxDuration: 299 } },
  { key: "from5to20", range: { minDuration: 300, maxDuration: 1199 } },
  { key: "from20to60", range: { minDuration: 1200, maxDuration: 3599 } },
  { key: "overHour", range: { minDuration: 3600 } },
];

/**
 * readWhole は URL の生の値（`getAll` の結果）の最初の値を 0 以上の整数として読む。無いか
 * 整数でなければ undefined。範囲は呼び手が確かめる。
 */
function readWhole(raw: readonly string[]): number | undefined | "invalid" {
  const value = raw[0];
  if (value === undefined) return undefined;
  if (!/^\d{1,9}$/.test(value)) return "invalid";
  return Number(value);
}

/** validAddedWithin は追加日の日数が範囲の中かを返す。 */
export function validAddedWithin(days: number): boolean {
  return Number.isInteger(days) && days >= MIN_ADDED_WITHIN && days <= MAX_ADDED_WITHIN;
}

/** validDuration は長さの秒数が範囲の中かを返す。 */
function validDuration(seconds: number): boolean {
  return Number.isInteger(seconds) && seconds >= 0 && seconds <= MAX_DURATION_SECONDS;
}

/**
 * validAddedLength は追加日と長さが送れる値か（範囲の中で、下限が上限を超えない）を返す。
 * サーバーは外れた値を `400` で断る。
 */
export function validAddedLength(filter: AddedLengthFilter): boolean {
  const { addedWithin, minDuration, maxDuration } = filter;
  if (addedWithin !== undefined && !validAddedWithin(addedWithin)) return false;
  if (minDuration !== undefined && !validDuration(minDuration)) return false;
  if (maxDuration !== undefined && !validDuration(maxDuration)) return false;
  return !(
    minDuration !== undefined &&
    maxDuration !== undefined &&
    minDuration > maxDuration
  );
}

/** ParsedAddedLength は URL から読んだ追加日と長さである。 */
export interface ParsedAddedLength {
  filter: AddedLengthFilter;
  /**
   * URL に送れない値（範囲の外、整数でない、下限が上限を超える）があった。画面は
   * `filter` の値で履歴を増やさずに URL を書き直す（ui-design.md「Added and length」）。
   */
  dropped: boolean;
}

/**
 * parseAddedLength は URL の問い合わせ部分から追加日と長さを読む。送れない値は落とす:
 * 範囲の外の値はその値だけを、下限が上限を超えるときは長さの両方を落とす。
 */
export function parseAddedLength(search: URLSearchParams): ParsedAddedLength {
  let dropped = false;
  const read = (name: string, valid: (value: number) => boolean) => {
    const value = readWhole(search.getAll(name));
    if (value === undefined) return undefined;
    if (value === "invalid" || !valid(value)) {
      dropped = true;
      return undefined;
    }
    return value;
  };
  const addedWithin = read(ADDED_WITHIN_PARAM, validAddedWithin);
  let minDuration = read(MIN_DURATION_PARAM, validDuration);
  let maxDuration = read(MAX_DURATION_PARAM, validDuration);
  if (
    minDuration !== undefined &&
    maxDuration !== undefined &&
    minDuration > maxDuration
  ) {
    dropped = true;
    minDuration = undefined;
    maxDuration = undefined;
  }
  return { filter: { addedWithin, minDuration, maxDuration }, dropped };
}

/**
 * useAddedLength は URL の問い合わせ部分から追加日と長さを読み、値が変わらない限り同じ
 * オブジェクトを返す。
 */
export function useAddedLength(search: string): ParsedAddedLength {
  const parsed = parseAddedLength(new URLSearchParams(search));
  const { addedWithin, minDuration, maxDuration } = parsed.filter;
  const filter = useMemo(
    () => ({ addedWithin, minDuration, maxDuration }),
    [addedWithin, minDuration, maxDuration],
  );
  return { filter, dropped: parsed.dropped };
}

/** hasAddedLength は追加日か長さで絞り込んでいるかを返す。 */
export function hasAddedLength(filter: AddedLengthFilter): boolean {
  return (
    filter.addedWithin !== undefined ||
    filter.minDuration !== undefined ||
    filter.maxDuration !== undefined
  );
}

/** addedLengthCount は絞り込みのボタンに数える数である（追加日と長さで 1 つずつ）。 */
export function addedLengthCount(filter: AddedLengthFilter): number {
  const added = filter.addedWithin === undefined ? 0 : 1;
  const length =
    filter.minDuration === undefined && filter.maxDuration === undefined ? 0 : 1;
  return added + length;
}

/** addedLengthSignature は絞り込みを値で比べるための文字列にする。 */
export function addedLengthSignature(filter: AddedLengthFilter): string {
  return [filter.addedWithin, filter.minDuration, filter.maxDuration]
    .map((value) => String(value ?? ""))
    .join("\u0000");
}

function written(value: number | undefined): string[] {
  return value === undefined ? [] : [String(value)];
}

/** withAddedWithin は追加日だけを差し替える `apply` の追加の値である。 */
export function withAddedWithin(days: number | undefined): Record<string, string[]> {
  return { [ADDED_WITHIN_PARAM]: written(days) };
}

/** withLength は長さの下限と上限だけを差し替える `apply` の追加の値である。 */
export function withLength(range: LengthRange): Record<string, string[]> {
  return {
    [MIN_DURATION_PARAM]: written(range.minDuration),
    [MAX_DURATION_PARAM]: written(range.maxDuration),
  };
}

/** addedLengthValues は絞り込みの全体を書く `apply` の追加の値である。 */
export function addedLengthValues(filter: AddedLengthFilter): Record<string, string[]> {
  return { ...withAddedWithin(filter.addedWithin), ...withLength(filter) };
}

/** clearedAddedLength は「Clear filters」で追加日と長さを外す `apply` の追加の値である。 */
export const clearedAddedLength: Readonly<Record<string, string[]>> = addedLengthValues(
  {},
);

/** AddedLengthOption は `Select` の選択肢 1 つである。 */
export interface AddedLengthOption<T> {
  /** `Select` の値。 */
  value: string;
  label: UiText;
  /** 選んだときの絞り込み。 */
  choice: T;
}

/** ANY_OPTION は `Any time`・`Any length` の値である。 */
const ANY_OPTION = "any";
/** CUSTOM_OPTION は選択肢に無い値（URL や条件が持つ値）の選択肢の値である。 */
const CUSTOM_OPTION = "custom";

/**
 * addedOptions は `Added` の選択肢を並べ、今の値の選択肢の値を返す。選択肢に無い日数は
 * `Last {N} days` として最後に足す（ほかを選ぶと消える）。
 */
export function addedOptions(current: number | undefined): {
  options: AddedLengthOption<number | undefined>[];
  value: string;
} {
  const words = t.list.filter.addedOptions;
  const options: AddedLengthOption<number | undefined>[] = [
    { value: ANY_OPTION, label: words.any, choice: undefined },
    ...addedPresets.map((days) => ({
      value: String(days),
      label: words.preset(days),
      choice: days,
    })),
  ];
  if (current === undefined) return { options, value: ANY_OPTION };
  if (addedPresets.includes(current)) return { options, value: String(current) };
  options.push({ value: CUSTOM_OPTION, label: words.days(current), choice: current });
  return { options, value: CUSTOM_OPTION };
}

function sameRange(a: LengthRange, b: LengthRange): boolean {
  return a.minDuration === b.minDuration && a.maxDuration === b.maxDuration;
}

/** minutes は秒数を分の表示にする（小数は 1 桁まで）。 */
function minutes(seconds: number): UiText {
  return formatNumber(Math.round(seconds / 6) / 10);
}

/** customLengthLabel は選択肢に無い長さの範囲の表示名である。 */
function customLengthLabel(range: LengthRange): UiText {
  const words = t.list.filter.lengthOptions;
  const { minDuration, maxDuration } = range;
  if (minDuration !== undefined && maxDuration !== undefined) {
    return words.between(minutes(minDuration), minutes(maxDuration));
  }
  if (maxDuration !== undefined) return words.under(minutes(maxDuration));
  return words.over(minutes(minDuration ?? 0));
}

/**
 * lengthOptions は `Length` の選択肢を並べ、今の値の選択肢の値を返す。選択肢に無い範囲は
 * その範囲の名前で最後に足す。
 */
export function lengthOptions(current: LengthRange): {
  options: AddedLengthOption<LengthRange>[];
  value: string;
} {
  const words = t.list.filter.lengthOptions;
  const options: AddedLengthOption<LengthRange>[] = [
    { value: ANY_OPTION, label: words.any, choice: {} },
    ...lengthPresets.map((entry) => ({
      value: entry.key,
      label: words[entry.key],
      choice: entry.range,
    })),
  ];
  const range: LengthRange = {
    minDuration: current.minDuration,
    maxDuration: current.maxDuration,
  };
  if (range.minDuration === undefined && range.maxDuration === undefined) {
    return { options, value: ANY_OPTION };
  }
  const preset = lengthPresets.find((entry) => sameRange(entry.range, range));
  if (preset !== undefined) return { options, value: preset.key };
  options.push({ value: CUSTOM_OPTION, label: customLengthLabel(range), choice: range });
  return { options, value: CUSTOM_OPTION };
}
