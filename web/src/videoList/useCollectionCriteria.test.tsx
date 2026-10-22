import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation, useNavigate } from "react-router";
import { afterEach, expect, it, vi } from "vitest";

import { AudienceProvider } from "../auth/audience";
import { useCollectionCriteria } from "./useCollectionCriteria";

afterEach(() => vi.restoreAllMocks());

it("検索・フィルターは保存順を保ち、一時ソート・解除・戻るでもグループの経路を保つ", async () => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <MemoryRouter initialEntries={["/groups/1?q=cat&trail=9"]}>
        <AudienceProvider audience="owner">{children}</AudienceProvider>
      </MemoryRouter>
    );
  }
  const { result } = renderHook(
    () => ({
      ...useCollectionCriteria(),
      location: useLocation(),
      navigate: useNavigate(),
    }),
    { wrapper: Wrapper },
  );
  expect(result.current.toolbar.nativeOrder.active).toBe(true);
  act(() => result.current.toolbar.onFavoriteChange(true));
  expect(new URLSearchParams(result.current.location.search).get("sort")).toBeNull();
  expect(result.current.conditions).toMatchObject({
    favorite: true,
    sort: undefined,
  });
  act(() => result.current.toolbar.onSortChange("titleDesc"));
  expect(result.current.criteria).toMatchObject({
    query: "cat",
    favorite: true,
    sort: "titleDesc",
  });
  act(() => result.current.toolbar.onClear());
  expect(result.current.criteria).toMatchObject({
    query: "",
    favorite: false,
    sort: "titleDesc",
  });
  act(() => {
    void result.current.navigate(-1);
  });
  await waitFor(() => expect(result.current.criteria.favorite).toBe(true));
  expect(new URLSearchParams(result.current.location.search).get("trail")).toBe("9");
  act(() => result.current.toolbar.nativeOrder.onSelect());
  expect(result.current.toolbar.nativeOrder.active).toBe(true);
  expect(result.current.conditions?.sort).toBeUndefined();
});

it("同じ描画から連続して操作しても検索・フィルター・ソートを上書きしない", () => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <MemoryRouter initialEntries={["/groups/1?q=cat"]}>
        <AudienceProvider audience="owner">{children}</AudienceProvider>
      </MemoryRouter>
    );
  }
  const { result } = renderHook(useCollectionCriteria, { wrapper: Wrapper });
  const toolbar = result.current.toolbar;
  act(() => {
    toolbar.onSortChange("titleAsc");
    toolbar.onFavoriteChange(true);
  });
  expect(result.current.criteria).toMatchObject({
    query: "cat",
    sort: "titleAsc",
    favorite: true,
  });
});

it("ランダムの再シャッフルは条件を保ち、保存順へ戻すとseedも除く", () => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <MemoryRouter initialEntries={["/groups/1?trail=9&fav=1"]}>
        <AudienceProvider audience="owner">{children}</AudienceProvider>
      </MemoryRouter>
    );
  }
  const { result } = renderHook(
    () => ({ ...useCollectionCriteria(), location: useLocation() }),
    { wrapper: Wrapper },
  );
  act(() => result.current.toolbar.onSortChange("random"));
  const firstSeed = result.current.conditions?.seed;
  expect(firstSeed).toBeGreaterThan(0);
  expect(result.current.customOrder).toBe(true);
  act(() => result.current.toolbar.onShuffle());
  expect(result.current.conditions).toMatchObject({ favorite: true, sort: "random" });
  expect(result.current.conditions?.seed).not.toBe(firstSeed);
  expect(new URLSearchParams(result.current.location.search).get("seed")).toBe(
    String(result.current.conditions?.seed),
  );
  act(() => result.current.toolbar.nativeOrder.onSelect());
  const params = new URLSearchParams(result.current.location.search);
  expect(params.has("sort")).toBe(false);
  expect(params.has("seed")).toBe(false);
  expect(params.get("trail")).toBe("9");
  expect(result.current.conditions).toMatchObject({
    favorite: true,
    seed: undefined,
    sort: undefined,
  });
  expect(result.current.customOrder).toBe(false);
});
