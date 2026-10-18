import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Reload the module per test because hasMounted is module state.
async function loadArrival() {
  vi.resetModules();
  return (await import("./useArrival")).useArrival;
}

describe("useArrival", () => {
  beforeEach(() => {
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("到着ごとに1回だけ先頭へ戻し、最初の表示のあとの到着でだけ見出しへフォーカスする", async () => {
    const useArrival = await loadArrival();
    const focused: string[] = [];
    function Folder({ name, restoring }: { name: string; restoring: boolean }) {
      const heading = useArrival(restoring);
      return (
        <h1 ref={heading} tabIndex={-1} onFocus={() => focused.push(name)}>
          {name}
        </h1>
      );
    }

    const { rerender } = render(<Folder key="a" name="a" restoring={false} />);
    expect(window.scrollTo).toHaveBeenCalledTimes(1);
    expect(focused).toEqual([]);

    rerender(<Folder key="a" name="a" restoring />);
    rerender(<Folder key="a" name="a" restoring={false} />);
    expect(window.scrollTo).toHaveBeenCalledTimes(1);
    expect(focused).toEqual([]);

    rerender(<Folder key="b" name="b" restoring={false} />);
    expect(window.scrollTo).toHaveBeenCalledTimes(2);
    expect(focused).toEqual(["b"]);
    rerender(<Folder key="b" name="b" restoring={false} />);
    expect(window.scrollTo).toHaveBeenCalledTimes(2);
    expect(focused).toEqual(["b"]);
  });

  it("控えから戻ったとき（restoring）はスクロールもフォーカスもしない", async () => {
    const useArrival = await loadArrival();
    const focused: string[] = [];
    function Folder({ name, restoring }: { name: string; restoring: boolean }) {
      const heading = useArrival(restoring);
      return (
        <h1 ref={heading} tabIndex={-1} onFocus={() => focused.push(name)}>
          {name}
        </h1>
      );
    }

    const { rerender } = render(<Folder key="a" name="a" restoring={false} />);
    rerender(<Folder key="b" name="b" restoring />);
    expect(window.scrollTo).toHaveBeenCalledTimes(1);
    expect(focused).toEqual([]);
  });
});
