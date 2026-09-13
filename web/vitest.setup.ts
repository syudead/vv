// テストごとに描画した DOM を片付ける。
//
// @testing-library/jest-dom は入れない。表明は Vitest の expect だけで書く
// （実行時にも開発時にも依存を増やさないため。specs/004-library-ui/tasks.md T003）。
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(() => {
  cleanup();
});
