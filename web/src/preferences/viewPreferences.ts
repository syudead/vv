import type { VideoSort } from "../api/client";

/**
 * Density は一覧の表示密度である（FR-017）。値と最小列幅の対応は
 * contracts/design-tokens.md 3. にある。
 *
 * サーバーは知らない列挙で、本機能が新しく決めるものである（data-model.md 1.）。
 */
export type Density = "dense" | "standard" | "relaxed";

/** ViewPreferences は利用者がその端末で選んだ一覧の見せ方である。 */
export interface ViewPreferences {
  /** 一覧の表示密度（FR-017）。 */
  density: Density;
  /**
   * 並び順の**初期値**である。いま表示している並び順そのものではない
   * （それは URL のクエリが持つ。data-model.md 1.「並び順の 2 つの役割」）。
   */
  sort: VideoSort;
}

/**
 * storageKey は保存先の鍵である（contracts/view-preferences.md 1.）。
 *
 * 版（`v1`）を鍵に含めるのは、項目の意味を変えるときに移行コードを書かずに
 * 済ませるためである。新しい鍵にすれば古い値は読まれず、設定は既定へ戻る —
 * FR-019 がその状態を正常と定めている（R-407）。
 */
const storageKey = "vv.view.v1";

/** defaults は読み出せない・壊れているときに返す値である（FR-019）。 */
const defaults: ViewPreferences = { density: "standard", sort: "addedDesc" };

/** densities は density として受け付ける値である。 */
const densities: Record<Density, true> = { dense: true, standard: true, relaxed: true };

/**
 * sorts は sort として受け付ける値である。
 *
 * 列挙を配列ではなく `Record<VideoSort, true>` で持つのは、`api/openapi.yaml`
 * に並び順が増えたときに**ここが型検査で落ちる**ようにするためである
 * （data-model.md 1.）。配列で持つと足りなくても型検査は通り、新しい並び順が
 * 黙って既定値へ落とされる。
 */
const sorts: Record<VideoSort, true> = { addedDesc: true, titleAsc: true };

/**
 * isKeyOf は値が表の鍵かどうかを返す。
 *
 * `in` ではなく `Object.hasOwn` を使う。`in` は原型の鎖まで見るので、
 * 保存された値が `"toString"` だったときに「正しい値」として通ってしまう。
 */
function isKeyOf<T extends string>(table: Record<T, true>, value: unknown): value is T {
  return typeof value === "string" && Object.hasOwn(table, value);
}

/**
 * readViewPreferences は表示設定を読む。**決して投げない**（契約 2.）。
 *
 * 判定は contracts/view-preferences.md 3. の 7 段を上から行う。5 と 6 が
 * **項目ごと**であることが要点で、片方が壊れただけで利用者の選択を丸ごと
 * 捨てない。壊れた値で画面が出ないのは最悪の失敗なので、検証はここ 1 か所に
 * 集め、画面はこの関数以外から localStorage に触らない（R-407）。
 *
 * @param storage 省略すると window.localStorage を使う。単体テストが偽の
 *   実装（投げる・満杯・壊れた値を返す）を渡せるようにするための引数である。
 */
export function readViewPreferences(storage?: Storage): ViewPreferences {
  let raw: string | null;
  try {
    // 参照そのものが投げる環境（プライベートウィンドウ）があるので、
    // localStorage を取り出すところから try の中に入れる（契約 3. の 1）。
    raw = (storage ?? window.localStorage).getItem(storageKey);
  } catch {
    return { ...defaults };
  }

  if (raw === null) {
    return { ...defaults };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ...defaults };
  }

  // 配列も typeof では "object" なので別に弾く（契約 3. の 4）。
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return { ...defaults };
  }

  const value = parsed as Record<string, unknown>;
  return {
    density: isKeyOf(densities, value.density) ? value.density : defaults.density,
    sort: isKeyOf(sorts, value.sort) ? value.sort : defaults.sort,
  };
}

/**
 * writeViewPreferences は表示設定を書く。**決して投げない**（契約 2.）。
 *
 * 書くのは上の 2 項目だけで、読んだときに残っていた未知の項目は引き継がない。
 * 引き継ぐと、他の版が書いた壊れた値を永久に運び続けることになる（契約 1.）。
 *
 * 失敗は黙って捨てる。「保存できませんでした」とは知らせない — 利用者に取れる
 * 対処が無く、画面上の設定はその場では効いたままでよい（契約 4.）。
 */
export function writeViewPreferences(value: ViewPreferences, storage?: Storage): void {
  try {
    const stored: ViewPreferences = { density: value.density, sort: value.sort };
    (storage ?? window.localStorage).setItem(storageKey, JSON.stringify(stored));
  } catch {
    // 設定が保存できないことは、画面を止める理由にならない（FR-019）。
  }
}
