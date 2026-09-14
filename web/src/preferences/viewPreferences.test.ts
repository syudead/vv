import { describe, expect, it, vi } from "vitest";

import { readViewPreferences, writeViewPreferences } from "./viewPreferences";

/**
 * 表示設定の読み書き（contracts/view-preferences.md 3.・4. / FR-019 / SC-007）。
 *
 * 要点は 2 つある。**どの場合でも投げず完全な値が返る**こと（壊れた値で画面が
 * 出ないのは最悪の失敗である）と、**項目ごとに既定値へ落とす**ことである。
 * 後者を外すと、density が壊れただけで利用者の選んだ並び順まで捨てられる。
 */

/** storageKey は viewPreferences.ts と同じ鍵である。 */
const storageKey = "vv.view.v1";

/**
 * fake は偽の Storage を作る。getItem の中身だけを差し替え、ほかは使わない。
 *
 * Storage の完全な実装は要らないので、必要な 2 つだけを持たせて型を合わせる。
 */
function fake(getItem: () => string | null, setItem: () => void = () => {}): Storage {
  return {
    getItem,
    setItem,
    removeItem: () => {},
    clear: () => {},
    key: () => null,
    length: 0,
  } as Storage;
}

/** throws は呼ぶと投げる関数である。 */
function throws(): never {
  throw new Error("localStorage は使えません");
}

describe("readViewPreferences", () => {
  // 契約 3. の 7 段を上から順に確かめる。どの段でも例外が外へ出ないことが
  // 同時の表明である（投げればテストは失敗する）。

  it("1. getItem が投げても既定値を返す", () => {
    expect(readViewPreferences(fake(throws))).toEqual({
      density: "standard",
      sort: "addedDesc",
    });
  });

  it("2. 鍵が無ければ既定値を返す", () => {
    expect(readViewPreferences(fake(() => null))).toEqual({
      density: "standard",
      sort: "addedDesc",
    });
  });

  it("3. JSON として読めなければ既定値を返す", () => {
    expect(readViewPreferences(fake(() => "{ density:"))).toEqual({
      density: "standard",
      sort: "addedDesc",
    });
  });

  it("4. オブジェクトでなければ既定値を返す", () => {
    // 配列・null・数値・文字列のいずれも「オブジェクト 1 個」ではない。
    // 配列は typeof で "object" になるので、別に弾けているかを確かめる。
    for (const raw of ["[]", "null", "42", '"standard"']) {
      expect(readViewPreferences(fake(() => raw))).toEqual({
        density: "standard",
        sort: "addedDesc",
      });
    }
  });

  it("5. density だけが壊れていれば density だけを既定値へ落とす", () => {
    const raw = JSON.stringify({ density: "huge", sort: "titleAsc" });

    // sort は活かす。片方が壊れただけで利用者の選択を丸ごと捨てない。
    expect(readViewPreferences(fake(() => raw))).toEqual({
      density: "standard",
      sort: "titleAsc",
    });
  });

  it("6. sort だけが壊れていれば sort だけを既定値へ落とす", () => {
    const raw = JSON.stringify({ density: "relaxed", sort: "sizeDesc" });

    expect(readViewPreferences(fake(() => raw))).toEqual({
      density: "relaxed",
      sort: "addedDesc",
    });
  });

  it("7. どちらも正しければそのまま使う", () => {
    const raw = JSON.stringify({ density: "dense", sort: "titleAsc" });

    expect(readViewPreferences(fake(() => raw))).toEqual({
      density: "dense",
      sort: "titleAsc",
    });
  });

  it("原型の鎖にある名前を正しい値として通さない", () => {
    // `in` で判定すると "toString" が通ってしまう。Object.hasOwn で見ている
    // ことの表明である。
    const raw = JSON.stringify({ density: "toString", sort: "constructor" });

    expect(readViewPreferences(fake(() => raw))).toEqual({
      density: "standard",
      sort: "addedDesc",
    });
  });
});

describe("writeViewPreferences", () => {
  it("setItem が投げても外へ出さない", () => {
    expect(() => {
      writeViewPreferences(
        { density: "dense", sort: "titleAsc" },
        fake(() => null, throws),
      );
    }).not.toThrow();
  });

  it("鍵と 2 項目だけを書く", () => {
    const setItem = vi.fn();

    writeViewPreferences(
      { density: "relaxed", sort: "titleAsc" },
      fake(() => null, setItem),
    );

    expect(setItem).toHaveBeenCalledTimes(1);
    expect(setItem.mock.calls[0]?.[0]).toBe(storageKey);
    expect(JSON.parse(String(setItem.mock.calls[0]?.[1]))).toEqual({
      density: "relaxed",
      sort: "titleAsc",
    });
  });

  it("読んだときに残っていた未知の項目を引き継がない", () => {
    // 他の版が書いた項目を運び続けると、壊れた値が永久に残る（契約 1.）。
    const raw = JSON.stringify({ density: "dense", sort: "titleAsc", theme: "light" });
    const setItem = vi.fn();

    const read = readViewPreferences(fake(() => raw));
    writeViewPreferences(
      read,
      fake(() => raw, setItem),
    );

    expect(JSON.parse(String(setItem.mock.calls[0]?.[1]))).toEqual({
      density: "dense",
      sort: "titleAsc",
    });
  });
});
