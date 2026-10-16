import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import {
  autoImportSettings as settings,
  autoImportURL,
  deferred,
  installAutoImportServer,
  json,
  renderWithScan,
} from "../testing/autoImportServer";
import AutoImportSwitch from "./AutoImportSwitch";
import { useAutoImportSettings } from "./useAutoImportSettings";

/** 切り替えだけを、開いたときの読み込みで出す。 */
function SwitchHarness() {
  const source = useAutoImportSettings();
  return <AutoImportSwitch source={source} />;
}

function autoImportSwitch(): HTMLElement {
  return screen.getByRole("switch", { name: "Pick up changes as they happen" });
}

describe("AutoImportSwitch", () => {
  const server = installAutoImportServer();

  it("shows the saved choice", async () => {
    server.current = settings(false);
    const view = renderWithScan(<SwitchHarness />);
    await screen.findByRole("switch");
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("false");
    view.unmount();

    server.current = settings(true);
    renderWithScan(<SwitchHarness />);
    await screen.findByRole("switch");
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("true");
  });

  it("saves the choice, ignores a second press while saving, and settles", async () => {
    server.current = settings(false);
    const user = userEvent.setup();
    renderWithScan(<SwitchHarness />);
    await screen.findByRole("switch");

    const put = deferred<Response>();
    server.fetchMock.mockImplementation((input, init) =>
      init?.method === "PUT" && String(input) === autoImportURL
        ? put.promise
        : server.route(input, init),
    );
    await user.click(autoImportSwitch());

    expect(server.fetchMock).toHaveBeenCalledWith(
      autoImportURL,
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ enabled: true }),
      }),
    );
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("true");
    expect(autoImportSwitch().getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Saving…")).toBeDefined();
    // 送信中にもう一度押しても送らない。
    await user.click(autoImportSwitch());
    expect(server.puts()).toBe(1);

    await act(async () => put.resolve(json(settings(true, { state: "starting" }))));
    expect(screen.queryByText("Saving…")).toBeNull();
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("true");
    expect(autoImportSwitch().getAttribute("aria-disabled")).toBeNull();
  });

  it("reverts the switch and says why when saving fails", async () => {
    server.current = settings(false);
    const user = userEvent.setup();
    renderWithScan(<SwitchHarness />);
    await screen.findByRole("switch");
    server.fetchMock.mockImplementation((input, init) =>
      init?.method === "PUT" && String(input) === autoImportURL
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : server.route(input, init),
    );

    await user.click(autoImportSwitch());

    const alert = await screen.findByText(/Couldn't change the setting:/);
    expect(alert.closest("[data-slot='alert']")?.getAttribute("role")).toBe("alert");
    await waitFor(() =>
      expect(autoImportSwitch().getAttribute("aria-checked")).toBe("false"),
    );
  });

  it("offers a retry when the first read fails", async () => {
    server.fetchMock.mockImplementation((input, init) =>
      String(input) === autoImportURL
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : server.route(input, init),
    );
    const user = userEvent.setup();
    renderWithScan(<SwitchHarness />);

    expect(
      await screen.findByText(/Couldn't load the auto-import settings:/),
    ).toBeDefined();
    expect(screen.queryByRole("switch")).toBeNull();

    server.fetchMock.mockImplementation((input, init) => server.route(input, init));
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByRole("switch");
    expect(autoImportSwitch().getAttribute("aria-checked")).toBe("true");
  });

  it("does not follow the watch: a regained focus reads nothing", async () => {
    renderWithScan(<SwitchHarness />);
    await screen.findByRole("switch");
    const initial = server.gets();

    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });
    expect(server.gets()).toBe(initial);
  });
});
