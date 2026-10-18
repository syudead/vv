import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { readViewPreferences } from "../preferences/viewPreferences";
import { useViewControls } from "./pageHooks";

afterEach(() => {
  localStorage.clear();
});

describe("useViewControls", () => {
  it("大きさと表示形式を変えると、ホバープレビューを止めてライブラリと共有の設定に残す", () => {
    const resetPreview = vi.fn();
    const { result } = renderHook(() => useViewControls(resetPreview));
    const before = result.current.zoom;
    const next = before === 1 ? 2 : 1;

    act(() => result.current.changeZoom(next));
    expect(result.current.zoom).toBe(next);
    expect(readViewPreferences().zoom).toBe(next);
    expect(resetPreview).toHaveBeenCalledTimes(1);

    act(() => result.current.changeView("list"));
    expect(result.current.view).toBe("list");
    expect(readViewPreferences()).toMatchObject({ view: "list", zoom: next });
    expect(resetPreview).toHaveBeenCalledTimes(2);
  });
});
