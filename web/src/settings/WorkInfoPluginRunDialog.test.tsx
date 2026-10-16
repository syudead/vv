import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { WorkInfoPlugin, WorkInfoPluginRun } from "../api/workInfoPlugins";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import WorkInfoPluginsSection from "./WorkInfoPluginsSection";

// 設定の「Work-info plugins」節で、行のメニューから直近の実行の出力を開く流れのページのテストである
// （specs/056-plugin-tooling/ui-design.md「Last run dialog」）。失敗の理由の文は
// workInfoPluginRows.test.ts が確かめる。

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const ranAt = 1_800_000_000;

function plugin(values: Partial<WorkInfoPlugin> & { id: number }): WorkInfoPlugin {
  return {
    name: "example.works",
    fileName: "example.wasm",
    version: "1.2.0",
    operations: ["search", "fetch"],
    digest: "a".repeat(64),
    state: "approved",
    settings: [],
    network: false,
    updatedAt: ranAt,
    ...values,
  };
}

function run(values: Partial<WorkInfoPluginRun> = {}): WorkInfoPluginRun {
  return {
    ranAt,
    operation: "fetch",
    stderr: "fetch: token=[secret]\n",
    truncated: false,
    ...values,
  };
}

/** LastRun は偽のサーバーが last-run に返す答えで、undefined は 500 の失敗である。 */
type LastRun = WorkInfoPluginRun | null | undefined;

/** server は一覧と、呼ばれるたびに次の last-run の答えを返す偽のサーバーである。 */
function server(plugins: WorkInfoPlugin[], lastRuns: LastRun[]) {
  const reads: string[] = [];
  const handler = (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input);
    if (url === "/api/work-info-plugins") return Promise.resolve(json({ plugins }));
    if (/^\/api\/work-info-plugins\/\d+\/last-run$/.test(url)) {
      reads.push(url);
      const answer = lastRuns[Math.min(reads.length, lastRuns.length) - 1];
      if (answer === undefined)
        return Promise.resolve(json({ code: "internal", message: "boom" }, 500));
      return Promise.resolve(json(answer === null ? {} : { run: answer }));
    }
    throw new Error(`unexpected request: ${url}`);
  };
  return { handler, reads };
}

function renderSection() {
  return render(
    <MemoryRouter initialEntries={["/settings"]}>
      <TooltipProvider>
        <ToastProvider>
          <WorkInfoPluginsSection />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

async function openLastRun(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(
    await screen.findByRole("button", { name: `More actions for ${name}` }),
  );
  await user.click(await screen.findByRole("menuitem", { name: "Last run output" }));
  return screen.getByRole("dialog", { name: `Last run of ${name}` });
}

describe("WorkInfoPluginRunDialog", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("行は直近の実行の時刻を示し、メニューから実行の事実と伏せた出力を開く", async () => {
    const fake = server([plugin({ id: 4, lastRunAt: ranAt })], [run()]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    const when = await screen.findByText(/^Last run /);
    expect(when.getAttribute("title")).not.toBe("");
    const dialog = await openLastRun(user, "example.works");

    const output = await within(dialog).findByRole("region", { name: "Output" });
    expect(output.textContent).toBe("fetch: token=[secret]\n");
    expect(within(dialog).getByText("Ran")).toBeDefined();
    expect(within(dialog).getByText("Fetch by ID or URL")).toBeDefined();
    expect(within(dialog).getByText("Finished")).toBeDefined();
    expect(within(dialog).queryByText("The output was cut at 64 KB.")).toBeNull();
    expect(fake.reads).toEqual(["/api/work-info-plugins/4/last-run"]);

    // 項目はメニューと一緒に消えるので、閉じたらその行の「More actions」へ戻る。
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const more = screen.getByRole("button", { name: "More actions for example.works" });
    await waitFor(() => expect(document.activeElement).toBe(more));

    await openLastRun(user, "example.works");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(more));
  });

  it("64 KiB の出力は折り返してその区画の中だけで動かし、切り詰めを注記する", async () => {
    const long = `${"x".repeat(300)}\n`.repeat(218);
    fetchMock.mockImplementation(
      server(
        [plugin({ id: 4, lastRunAt: ranAt })],
        [run({ stderr: long, truncated: true })],
      ).handler,
    );
    const user = userEvent.setup();
    renderSection();

    const dialog = await openLastRun(user, "example.works");

    const output = await within(dialog).findByRole("region", { name: "Output" });
    expect(output.textContent).toBe(long);
    // 長い行は折り返し、高さは max-h-run-output に抑えてこの区画だけを動かすので、ダイアログも
    // ページも広がらない。ダイアログは FormDialog の wide の幅に留まる。
    for (const name of [
      "whitespace-pre-wrap",
      "break-all",
      "max-h-run-output",
      "overflow-y-auto",
      "font-mono",
      "text-xs",
    ])
      expect(output.classList).toContain(name);
    expect(dialog.className).toContain("max-w-2xl");
    expect(within(dialog).getByText("The output was cut at 64 KB.")).toBeDefined();
  });

  it("実行が無ければそう言い、何も書かなかった実行は事実と書かなかったことを示す", async () => {
    fetchMock.mockImplementation(
      server(
        [plugin({ id: 4 })],
        [null, run({ stderr: "", operation: "search", failure: "network_error" })],
      ).handler,
    );
    const user = userEvent.setup();
    renderSection();

    let dialog = await openLastRun(user, "example.works");
    expect(
      await within(dialog).findByText(
        "No run yet. Run the plugin from a video page, then open this again.",
      ),
    ).toBeDefined();
    expect(within(dialog).queryByText("Ran")).toBeNull();
    expect(within(dialog).queryByRole("region", { name: "Output" })).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    dialog = await openLastRun(user, "example.works");
    expect(await within(dialog).findByText("The last run wrote nothing.")).toBeDefined();
    expect(within(dialog).getByText("Search")).toBeDefined();
    expect(
      within(dialog).getByText("Failed: The plugin couldn't reach a site."),
    ).toBeDefined();
    expect(within(dialog).queryByRole("region", { name: "Output" })).toBeNull();
  });

  it("開くたびに読み直すので、2 回目の取得の出力が前の出力に置き換わる", async () => {
    fetchMock.mockImplementation(
      server(
        [plugin({ id: 4, lastRunAt: ranAt })],
        [run({ stderr: "first run\n" }), run({ stderr: "second run\n" })],
      ).handler,
    );
    const user = userEvent.setup();
    renderSection();

    let dialog = await openLastRun(user, "example.works");
    expect(
      (await within(dialog).findByRole("region", { name: "Output" })).textContent,
    ).toBe("first run\n");
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    dialog = await openLastRun(user, "example.works");
    await waitFor(() =>
      expect(within(dialog).getByRole("region", { name: "Output" }).textContent).toBe(
        "second run\n",
      ),
    );
  });

  it("読み込みの失敗は Retry で読み直し、使えないプラグインにはメニューの項目が無い", async () => {
    fetchMock.mockImplementation(
      server(
        [
          plugin({ id: 4 }),
          plugin({
            id: 5,
            name: undefined,
            fileName: "broken.wasm",
            version: undefined,
            operations: [],
            state: "invalid",
            problem: "not_wasm",
          }),
        ],
        [undefined, run()],
      ).handler,
    );
    const user = userEvent.setup();
    renderSection();

    const dialog = await openLastRun(user, "example.works");
    expect(await within(dialog).findByText(/^Couldn't load the last run:/)).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    expect(await within(dialog).findByRole("region", { name: "Output" })).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(
      screen.getByRole("button", { name: "More actions for broken.wasm" }),
    );
    expect(await screen.findByRole("menuitem", { name: "Delete" })).toBeDefined();
    expect(screen.queryByRole("menuitem", { name: "Last run output" })).toBeNull();
  });

  it("ダイアログの文言はカタログか利用者のデータだけである", async () => {
    enablePseudoLocale();
    fetchMock.mockImplementation(
      server(
        [plugin({ id: 4, lastRunAt: ranAt })],
        [run({ truncated: true, failure: "plugin_failed" })],
      ).handler,
    );
    const user = userEvent.setup();
    renderSection();

    await screen.findByText("example.works");
    // 疑似ロケールでは文言が飾られるので、行の唯一のボタンとメニューの最初の項目で開く。
    await user.click(screen.getByRole("button", { name: /example\.works/ }));
    const [lastRunItem] = await screen.findAllByRole("menuitem");
    if (lastRunItem === undefined) throw new Error("no menu item");
    await user.click(lastRunItem);
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByRole("region");
    expectCatalogTextOnly(dialog, ["example.works", "fetch: token=[secret]", "1.2.0"]);
  });
});
