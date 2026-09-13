// テストごとに描画した DOM を片付ける。
//
// @testing-library/jest-dom は入れない。表明は Vitest の expect だけで書く
// （実行時にも開発時にも依存を増やさないため。specs/004-library-ui/tasks.md T003）。
import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";

afterEach(() => {
  cleanup();
});

// Testing Library は各操作のあと setTimeout(..., 0) でマイクロタスクを吐き出し、
// **偽の時計が動いているときだけ**それを自分で進める。その判定が jest の
// 名前を見に行くので、Vitest の偽の時計では待ちが永久に明けない
// （user-event で打鍵する検査が待ち合わせを確かめられなくなる）。
// 進め方だけを渡して判定を通す。時計そのものは Vitest のままである。
Object.assign(globalThis, {
  jest: { advanceTimersByTime: vi.advanceTimersByTime.bind(vi) },
});
