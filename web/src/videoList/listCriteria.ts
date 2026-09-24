import {
  isVideoSort,
  MAX_QUERY_LENGTH,
  videoSorts,
  type VideoSort,
  type WatchFilter,
} from "../api/client";

/**
 * 一覧の条件と URL の相互変換（specs/013-library-search/contracts/list-url.md）。
 *
 * ライブラリとフォルダ画面は同じクエリパラメータで条件を表す（Plan の
 * Structural Decisions 9）。ここは React に依存しない純粋な関数だけを置き、
 * 画面との結び付けは useListCriteria が受け持つ。
 */

/** ListCriteria は一覧を決める条件である。 */
export interface ListCriteria {
  /** 検索語。前後の空白を落とし、100 符号位置で切ったもの。空なら検索しない。 */
  query: string;
  watch: WatchFilter;
  /** true なら再生できるものだけにする。 */
  playable: boolean;
  sort: VideoSort;
  /** sort=random の並びを決める値。ほかの並び順では常に undefined。 */
  seed?: number;
}

/** MAX_SEED は seed に許す最大値である（list-url.md §1）。 */
export const MAX_SEED = 2147483647;

// 並び順の一覧は api/client が持つ。画面の部品からはここ経由でも使えるようにする。
export { isVideoSort, videoSorts };

/** DEFAULT_SORT は端末に何も保存していないときの並び順である。 */
export const DEFAULT_SORT: VideoSort = "addedDesc";

const watchValues: readonly WatchFilter[] = ["all", "unwatched", "inProgress", "watched"];

/** SortKind は並べ替えの種類である（向きを除いたもの）。 */
export type SortKind =
  "added" | "modified" | "title" | "duration" | "size" | "played" | "random";

export type SortDirection = "asc" | "desc";

export interface SortKindInfo {
  kind: SortKind;
  label: string;
  /** 種類を選んだときの並び順（ui-design.md「Sort and direction」の表）。 */
  initial: VideoSort;
  /** 昇順と降順の値。ランダムは向きを持たない。 */
  asc?: VideoSort;
  desc?: VideoSort;
  /** 昇順／降順の言い換え。無い種類は undefined。 */
  ascWording?: string;
  descWording?: string;
}

/** sortKinds はメニューに並べる7つの種類である。順はメニューの順と同じ。 */
export const sortKinds: readonly SortKindInfo[] = [
  {
    kind: "added",
    label: "追加日",
    initial: "addedDesc",
    asc: "addedAsc",
    desc: "addedDesc",
    ascWording: "古い順",
    descWording: "新しい順",
  },
  {
    kind: "modified",
    label: "更新日時",
    initial: "modifiedDesc",
    asc: "modifiedAsc",
    desc: "modifiedDesc",
    ascWording: "古い順",
    descWording: "新しい順",
  },
  {
    kind: "title",
    label: "題名",
    initial: "titleAsc",
    asc: "titleAsc",
    desc: "titleDesc",
  },
  {
    kind: "duration",
    label: "長さ",
    initial: "durationDesc",
    asc: "durationAsc",
    desc: "durationDesc",
    ascWording: "短い順",
    descWording: "長い順",
  },
  {
    kind: "size",
    label: "ファイルサイズ",
    initial: "sizeDesc",
    asc: "sizeAsc",
    desc: "sizeDesc",
    ascWording: "小さい順",
    descWording: "大きい順",
  },
  {
    kind: "played",
    label: "最近再生した順",
    initial: "playedDesc",
    asc: "playedAsc",
    desc: "playedDesc",
    ascWording: "前に再生した順",
    descWording: "最近再生した順",
  },
  { kind: "random", label: "ランダム", initial: "random" },
];

/** sortKindOf は並び順の種類の情報を返す。 */
export function sortKindOf(sort: VideoSort): SortKindInfo {
  const found = sortKinds.find((info) => info.asc === sort || info.desc === sort);
  // random は asc・desc を持たないので、見つからなければランダムである。
  return found ?? (sortKinds[sortKinds.length - 1] as SortKindInfo);
}

/** sortDirection は並び順の向きを返す。ランダムは undefined。 */
export function sortDirection(sort: VideoSort): SortDirection | undefined {
  if (sort === "random") return undefined;
  return sort.endsWith("Asc") ? "asc" : "desc";
}

/** withDirection は同じ種類で向きだけを変えた並び順を返す。ランダムはそのまま。 */
export function withDirection(sort: VideoSort, direction: SortDirection): VideoSort {
  const info = sortKindOf(sort);
  return (direction === "asc" ? info.asc : info.desc) ?? sort;
}

/**
 * directionLabel は向きの読み上げ名を作る（ui-design.md「Sort and direction」）。
 * 例: 「降順（新しい順）」。言い換えの無い種類は「降順」。
 */
export function directionLabel(sort: VideoSort, direction: SortDirection): string {
  const info = sortKindOf(sort);
  const wording = direction === "asc" ? info.ascWording : info.descWording;
  const name = direction === "asc" ? "昇順" : "降順";
  return wording === undefined ? name : `${name}（${wording}）`;
}

/**
 * directionToggleLabel は向きの切り替えボタンの読み上げ名とツールチップである。
 * 例: 「降順（新しい順）。押すと昇順」。
 */
export function directionToggleLabel(sort: VideoSort): string {
  const current = sortDirection(sort) ?? "desc";
  const next = current === "asc" ? "降順" : "昇順";
  return `${directionLabel(sort, current)}。押すと${next}`;
}

/** normalizeQuery は検索語を URL と一覧の条件に使う形にする。 */
export function normalizeQuery(value: string): string {
  // 100 符号位置で切る（サーバーの []rune と同じ数え方。サロゲートペアを割らない）。
  return Array.from(value.trim()).slice(0, MAX_QUERY_LENGTH).join("").trim();
}

/** parseSeed は seed の値を読む。1 以上 MAX_SEED 以下の整数でなければ undefined。 */
export function parseSeed(value: string | null): number | undefined {
  if (value === null || !/^\d{1,10}$/.test(value)) return undefined;
  const seed = Number(value);
  return seed >= 1 && seed <= MAX_SEED ? seed : undefined;
}

/** newSeed は新しい seed を作る。previous と同じ値は避ける（並べ直すで並びを変える）。 */
export function newSeed(previous?: number, random: () => number = Math.random): number {
  for (;;) {
    const seed = 1 + Math.floor(random() * MAX_SEED);
    const bounded = Math.min(Math.max(seed, 1), MAX_SEED);
    if (bounded !== previous) return bounded;
  }
}

/** ParsedCriteria は URL を読んだ結果である。 */
export interface ParsedCriteria {
  criteria: ListCriteria;
  /** sort=random なのに seed が無いか壊れている。画面が seed を作って URL を置き換える。 */
  needsSeed: boolean;
  /** URL に解釈できる sort が書かれている。 */
  hasExplicitSort: boolean;
}

/**
 * parseListCriteria は URL のクエリを一覧の条件にする。
 *
 * 解釈できない値は既定として扱い、誤りを出さない。sort が無いか解釈できない
 * ときは、端末に保存した並び順（preferredSort）を使う。
 */
export function parseListCriteria(
  params: URLSearchParams,
  preferredSort: VideoSort = DEFAULT_SORT,
): ParsedCriteria {
  const rawSort = params.get("sort");
  const hasExplicitSort = isVideoSort(rawSort);
  const sort: VideoSort = hasExplicitSort ? rawSort : preferredSort;
  const rawWatch = params.get("watch");
  const watch =
    rawWatch !== null && (watchValues as readonly string[]).includes(rawWatch)
      ? (rawWatch as WatchFilter)
      : "all";
  const seed = sort === "random" ? parseSeed(params.get("seed")) : undefined;
  return {
    criteria: {
      query: normalizeQuery(params.get("q") ?? ""),
      watch,
      playable: params.get("playable") === "1",
      sort,
      ...(seed === undefined ? {} : { seed }),
    },
    needsSeed: sort === "random" && seed === undefined,
    hasExplicitSort,
  };
}

/**
 * serializeListCriteria は条件を URL のクエリにする。
 *
 * watch=all と playable の偽は書かない。sort は書く — 省略すると「端末に
 * 保存した並び順」の意味になり、並べ替えを変えたあとに戻るで前の並びに
 * 戻れなくなるからである（list-url.md §1）。seed は random のときだけ書く。
 * 順は q・watch・playable・sort・seed で固定し、同じ条件は同じ文字列になる。
 */
export function serializeListCriteria(criteria: ListCriteria): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  if (criteria.watch !== "all") params.set("watch", criteria.watch);
  if (criteria.playable) params.set("playable", "1");
  params.set("sort", criteria.sort);
  if (criteria.sort === "random" && criteria.seed !== undefined) {
    params.set("seed", String(criteria.seed));
  }
  return params;
}

/** hasConditions は「条件を解除」で外せる条件（検索語・視聴状態・再生可否）があるか。 */
export function hasConditions(criteria: ListCriteria): boolean {
  return criteria.query !== "" || criteria.watch !== "all" || criteria.playable;
}

/** clearConditions は検索語・視聴状態・再生可否を外す。並べ替えと seed は残す。 */
export function clearConditions(criteria: ListCriteria): ListCriteria {
  return { ...criteria, query: "", watch: "all", playable: false };
}

/** criteriaKey は条件を比較・依存配列に使う文字列にする。 */
export function criteriaKey(criteria: ListCriteria): string {
  return serializeListCriteria(criteria).toString();
}

/** HistoryMode は URL の書き換え方である。 */
export type HistoryMode = "push" | "replace";

/**
 * SearchSession は検索欄の一続きの入力が履歴をどう増やすかを決める（list-url.md §3）。
 *
 * フォーカスが入ってから外れるか Esc で抜けるまでを1つの続きとし、その中の
 * 最初の確定だけが履歴を1つ増やし、以後は置き換える。
 */
export class SearchSession {
  private pushed = false;

  /** start は新しい続きを始める（検索欄にフォーカスが入った）。 */
  start(): void {
    this.pushed = false;
  }

  /** commit は確定1回分の書き換え方を返す。 */
  commit(): HistoryMode {
    if (this.pushed) return "replace";
    this.pushed = true;
    return "push";
  }

  /** end は続きを閉じる（フォーカスが外れた・Esc で抜けた）。 */
  end(): void {
    this.pushed = false;
  }
}
