import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { deferredRoute } from "./deferredRoute";

function Page() {
  return <p>読み込んだ画面</p>;
}

describe("deferredRoute", () => {
  afterEach(() => window.history.replaceState({}, "", "/"));

  it("起動時の URL が当たるときは読み終えてから返し、最初の描画から画面を出す", async () => {
    window.history.replaceState({}, "", "/tags");
    const load = vi.fn(() => Promise.resolve({ default: Page }));
    const Route = await deferredRoute(load, "/tags");

    render(<Route />);

    expect(screen.getByText("読み込んだ画面")).toBeDefined();
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("アプリの中で移ってきたときは、読み込みが済んだところで画面を出す", async () => {
    let resolve: (module: { default: typeof Page }) => void = () => undefined;
    const load = vi.fn(
      () =>
        new Promise<{ default: typeof Page }>((done) => {
          resolve = done;
        }),
    );
    const Route = await deferredRoute(load, "/tags");
    expect(load).not.toHaveBeenCalled();

    render(<Route />);
    expect(screen.queryByText("読み込んだ画面")).toBeNull();

    resolve({ default: Page });
    expect(await screen.findByText("読み込んだ画面")).toBeDefined();
  });

  it("読み込めなければ空白にせず、再読み込みを促す", async () => {
    const reload = vi.fn();
    const Route = await deferredRoute(
      () => Promise.reject(new Error("chunk failed")),
      "/tags",
      reload,
    );

    render(<Route />);

    expect(
      await screen.findByRole("heading", { name: "画面を読み込めませんでした" }),
    ).toBeDefined();
    await userEvent.setup().click(screen.getByRole("button", { name: "再読み込み" }));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("起動時の読み込みに失敗しても、描画の側で読み直して画面を出せる", async () => {
    window.history.replaceState({}, "", "/folders/a");
    const load = vi
      .fn<() => Promise<{ default: typeof Page }>>()
      .mockRejectedValueOnce(new Error("chunk failed"))
      .mockResolvedValue({ default: Page });
    const Route = await deferredRoute(load, "/folders");

    render(<Route />);

    expect(await screen.findByText("読み込んだ画面")).toBeDefined();
    expect(load).toHaveBeenCalledTimes(2);
  });
});
