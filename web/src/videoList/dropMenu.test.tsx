import { act, renderHook } from "@testing-library/react";
import { useCallback } from "react";
import { describe, expect, it } from "vitest";

import { asUiText } from "../i18n/uiText";
import type { CardDrop } from "./cardDrag";
import { useDropMenu } from "./dropMenu";

// 落とした先のメニューを保つ印。

const drop: CardDrop = {
  lifted: ["a"],
  sign: { kind: "onto", key: "b", label: asUiText("Put in") },
};

/** renderMenu は、一覧 `present` にあるカードを落とした先として受けるメニューを描く。 */
function renderMenu(present: readonly string[]) {
  return renderHook(
    ({ keys }: { keys: readonly string[] }) =>
      useDropMenu(
        useCallback(({ sign }: CardDrop) => keys.includes(sign.key), [keys]),
        () => null,
      ),
    { initialProps: { keys: present } },
  );
}

describe("useDropMenu", () => {
  it("holds the drop while the target is present and lets go when the target leaves", () => {
    const { result, rerender } = renderMenu(["a", "b"]);

    let accepted = false;
    act(() => {
      accepted = result.current.hold(drop);
    });
    expect(accepted).toBe(true);
    expect(result.current.held).toEqual({ lifted: ["a"], sign: drop.sign });
    expect(result.current.attached?.key).toBe("b");

    // 別の画面の操作や知らせで一覧が変わり、落とした先のカードが消えた。
    rerender({ keys: ["a"] });

    expect(result.current.held).toBeNull();
    expect(result.current.attached).toBeNull();
  });

  it("does not hold a drop the screen does not accept", () => {
    const { result } = renderMenu(["a"]);

    let accepted = true;
    act(() => {
      accepted = result.current.hold(drop);
    });

    expect(accepted).toBe(false);
    expect(result.current.held).toBeNull();
  });
});
