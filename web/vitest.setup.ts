// テストごとに描画した DOM を片付ける。
//
// @testing-library/jest-dom は入れない。表明は Vitest の expect だけで書く
// （実行時にも開発時にも依存を増やさないため）。
import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";

afterEach(() => {
  cleanup();

  // 表示設定（vv.view.v1）は localStorage に残る。jsdom の localStorage は
  // ファイル内のすべての検査で共有されるので、片付けないと「密度を変えた」
  // 検査が後続の検査の初期値を書き換え、実行順で結果が変わる。
  //
  // DOM を使わない検査（対比の検査は node 環境で CSS をファイルとして読む）
  // には localStorage が無いので、有無を見てから片付ける。
  if (typeof localStorage !== "undefined" && typeof localStorage.clear === "function") {
    localStorage.clear();
  }
});

// Testing Library は各操作のあと setTimeout(..., 0) でマイクロタスクを吐き出し、
// **偽の時計が動いているときだけ**それを自分で進める。その判定が jest の
// 名前を見に行くので、Vitest の偽の時計では待ちが永久に明けない
// （user-event で打鍵する検査が待ち合わせを確かめられなくなる）。
// 進め方だけを渡して判定を通す。時計そのものは Vitest のままである。
Object.assign(globalThis, {
  jest: { advanceTimersByTime: vi.advanceTimersByTime.bind(vi) },
});

// Radix Slider は ResizeObserver を要る。jsdom には無いので空の実装を置く。
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}
