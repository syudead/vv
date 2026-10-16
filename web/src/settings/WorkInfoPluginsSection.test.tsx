import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { WorkInfoPlugin, WorkInfoPluginSetting } from "../api/workInfoPlugins";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { WORK_INFO_PLUGINS_GUIDE_URL } from "./docsLinks";
import WorkInfoPluginsSection from "./WorkInfoPluginsSection";

// 設定の「Work-info plugins」節のページのテストである（specs/049-work-info-plugins/ui-design.md
// 「Settings: Work-info plugins section」）。値の決まりは workInfoPluginRows.test.ts が確かめる。

function json(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function plugin(values: Partial<WorkInfoPlugin> & { id: number }): WorkInfoPlugin {
  return {
    name: "example.works",
    fileName: "example.wasm",
    version: "1.2.0",
    description: "Fetches titles from example.works",
    operations: ["search", "fetch"],
    digest: "a".repeat(64),
    state: "unapproved",
    settings: [],
    network: true,
    updatedAt: 1_800_000_000,
    ...values,
  };
}

/** uploads はファイル名ごとに、偽のサーバーが追加で返すプラグインである。 */
const uploads: Record<string, Omit<WorkInfoPlugin, "id">> = {
  "example.wasm": plugin({ id: 0 }),
  "broken.wasm": plugin({
    id: 0,
    name: undefined,
    fileName: "broken.wasm",
    version: undefined,
    description: undefined,
    operations: [],
    state: "invalid",
    problem: "not_wasm",
  }),
};

/** saved は偽のサーバーが値を保存した後の設定である。秘密の値は返さない。 */
function saved(
  setting: WorkInfoPluginSetting,
  value: string | null | undefined,
): WorkInfoPluginSetting {
  const { value: _old, staleValue: _stale, ...rest } = setting;
  if (value === null || value === undefined) return { ...rest, set: false };
  return setting.secret ? { ...rest, set: true } : { ...rest, set: true, value };
}

type Override = (method: string, url: string) => Response | undefined;

/** server は保存されたプラグインを持つ偽のサーバーである。override は先に答える。 */
function server(initial: WorkInfoPlugin[], override: Override = () => undefined) {
  let stored = [...initial];
  let nextId = 100;
  const requests: { method: string; url: string; body?: unknown }[] = [];
  const replace = (id: number, change: Partial<WorkInfoPlugin>) => {
    stored = stored.map((item) => (item.id === id ? { ...item, ...change } : item));
    return stored.find((item) => item.id === id);
  };
  const upload = (form: FormData) => {
    const file = form.get("module") as File;
    const base = uploads[file.name];
    if (base === undefined) throw new Error(`unexpected upload: ${file.name}`);
    const existing = stored.find(
      (item) => item.name !== undefined && item.name === base.name,
    );
    if (existing !== undefined)
      return json(replace(existing.id, { ...base, id: existing.id }));
    const created = { ...base, id: nextId++ };
    stored = [...stored, created];
    return json(created, 201);
  };
  const handler = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = String(input);
    const method = init?.method ?? "GET";
    const body =
      typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined;
    requests.push({ method, url, body });
    const answer = override(method, url);
    if (answer !== undefined) return Promise.resolve(answer);
    if (url === "/api/work-info-plugins" && method === "GET")
      return Promise.resolve(json({ plugins: stored }));
    if (url === "/api/work-info-plugins" && method === "POST")
      return Promise.resolve(upload(init?.body as FormData));
    const match = /^\/api\/work-info-plugins\/(\d+)(\/approval|\/settings)?$/.exec(url);
    if (match === null) throw new Error(`unexpected request: ${method} ${url}`);
    const id = Number(match[1]);
    if (match[2] === "/approval")
      return Promise.resolve(
        json(replace(id, { state: method === "PUT" ? "approved" : "unapproved" })),
      );
    if (match[2] === "/settings") {
      const { values } = body as { values: Record<string, string | null> };
      const current = stored.find((item) => item.id === id);
      const settings = (current?.settings ?? []).map((setting) =>
        Object.hasOwn(values, setting.key)
          ? saved(setting, values[setting.key])
          : setting,
      );
      return Promise.resolve(json(replace(id, { settings })));
    }
    stored = stored.filter((item) => item.id !== id);
    return Promise.resolve(json(null, 204));
  };
  return { handler, requests, stored: () => stored };
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

function wasm(name: string, size?: number): File {
  const file = new File([new Uint8Array([0, 0x61, 0x73, 0x6d])], name, {
    type: "application/wasm",
  });
  if (size !== undefined) Object.defineProperty(file, "size", { value: size });
  return file;
}

function choose(file: File) {
  fireEvent.change(screen.getByTestId("work-info-plugin-input"), {
    target: { files: [file] },
  });
}

function list() {
  return screen.getByLabelText("Added work-info plugins", { selector: "div" });
}

function rowOf(text: string): HTMLElement {
  const row = within(list()).getByText(text).closest<HTMLElement>("[data-plugin-id]");
  if (row === null) throw new Error(`no row for ${text}`);
  return row;
}

describe("WorkInfoPluginsSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    // Radix Select はポインタの捕捉を使う。jsdom には無いので何もしない実装を置く。
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("空の一覧を出し、案内は文書サイトを新しいタブで開く", async () => {
    fetchMock.mockImplementation(server([]).handler);
    renderSection();

    expect(await screen.findByText("No plugins yet")).toBeDefined();
    const guide = screen.getByRole("link", { name: /How plugins work/ });
    expect(guide.getAttribute("href")).toBe(WORK_INFO_PLUGINS_GUIDE_URL);
    expect(guide.getAttribute("target")).toBe("_blank");
    expect(document.getElementById("work-info-plugins")).not.toBeNull();
  });

  it("追加したプラグインが名前・版・インターネットの注意付きで未承認として出て、その行の最初の操作へ移る", async () => {
    const fake = server([]);
    fetchMock.mockImplementation(fake.handler);
    renderSection();
    await screen.findByText("No plugins yet");

    choose(wasm("example.wasm"));

    expect(
      await screen.findByText("Added example.works. Approve it to use it."),
    ).toBeDefined();
    const row = rowOf("example.works");
    expect(within(row).getByText("Not approved")).toBeDefined();
    expect(within(row).getByText("v1.2.0")).toBeDefined();
    expect(within(row).getByText("Reaches internet sites")).toBeDefined();
    expect(within(row).getByText(/Search · Fetch by URL/)).toBeDefined();
    const approve = within(row).getByRole("button", { name: "Review and approve" });
    await waitFor(() => expect(document.activeElement).toBe(approve));
    const posted = fake.requests.find((request) => request.method === "POST");
    expect(posted?.url).toBe("/api/work-info-plugins");
  });

  it("同じ名前のファイルを足すと差し替えとして知らせる", async () => {
    fetchMock.mockImplementation(server([plugin({ id: 1, state: "approved" })]).handler);
    renderSection();
    await screen.findByText("Approved");

    choose(wasm("example.wasm"));

    expect(
      await screen.findByText("Replaced example.works. Approve it again to use it."),
    ).toBeDefined();
    expect(within(rowOf("example.works")).getByText("Not approved")).toBeDefined();
  });

  it("差し替えかどうかは一覧ではなく応答の状態コードで決める", async () => {
    // 別のタブで足されて一覧にまだ無いプラグインでも、200 なら差し替えと知らせる。
    const replaced = plugin({ id: 7 });
    fetchMock.mockImplementation(
      server([], (method, url) =>
        method === "POST" && url === "/api/work-info-plugins"
          ? json(replaced, 200)
          : undefined,
      ).handler,
    );
    renderSection();
    await screen.findByText("No plugins yet");

    choose(wasm("example.wasm"));

    expect(
      await screen.findByText("Replaced example.works. Approve it again to use it."),
    ).toBeDefined();
  });

  it("承認の確認は事実と注意を出し、表示した digest を送って承認済みに変える", async () => {
    const digest = "b".repeat(64);
    const fake = server([
      plugin({
        id: 1,
        digest,
        settings: [
          { key: "apiKey", label: "API key", secret: true, required: true, set: true },
        ],
      }),
    ]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Review and approve" }));
    const dialog = screen.getByRole("dialog", { name: "Approve example.works?" });
    expect(within(dialog).getByText(digest)).toBeDefined();
    expect(within(dialog).getByText("example.wasm")).toBeDefined();
    expect(
      within(dialog).getByText("This plugin can reach any site on the internet"),
    ).toBeDefined();
    expect(within(dialog).getByText("API key")).toBeDefined();
    expect(within(dialog).getByText("Secret")).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "Approve" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const row = rowOf("example.works");
    expect(within(row).getByText("Approved")).toBeDefined();
    expect(within(row).queryByRole("button", { name: "Review and approve" })).toBeNull();
    expect(fake.requests).toContainEqual({
      method: "PUT",
      url: "/api/work-info-plugins/1/approval",
      body: { digest },
    });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(row).getByRole("button", { name: "Settings" }),
      ),
    );
  });

  it("確認の間に差し替えられたら、窓を開いたまま知らせて Approve を押せなくし、一覧を読み直す", async () => {
    const fake = server([plugin({ id: 1 })], (method, url) =>
      method === "PUT" && url.endsWith("/approval")
        ? json({ code: "plugin_changed", message: "changed" }, 409)
        : undefined,
    );
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Review and approve" }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Approve" }));

    expect(
      await within(dialog).findByText(
        "The plugin was replaced while this was open. Close this and review it again.",
      ),
    ).toBeDefined();
    expect(
      within(dialog).getByRole("button", { name: "Approve" }).hasAttribute("disabled"),
    ).toBe(true);
    await waitFor(() =>
      expect(fake.requests.filter((request) => request.method === "GET")).toHaveLength(2),
    );
  });

  it("使えないプラグインは理由を示し、承認の操作を持たない", async () => {
    fetchMock.mockImplementation(server([]).handler);
    renderSection();
    await screen.findByText("No plugins yet");

    choose(wasm("broken.wasm"));

    expect(
      await screen.findByText("Added broken.wasm, but it can't be used."),
    ).toBeDefined();
    const row = rowOf("broken.wasm");
    expect(within(row).getByText("Can't be used")).toBeDefined();
    expect(within(row).getByText("The file isn't a WebAssembly module.")).toBeDefined();
    expect(within(row).queryByRole("button", { name: "Review and approve" })).toBeNull();
    expect(within(row).queryByText("Reaches internet sites")).toBeNull();
    expect(within(row).getAllByRole("button")).toHaveLength(1);
  });

  it("秘密の設定を保存すると設定済みとだけ出し、値を画面に戻さず、承認は変わらない", async () => {
    const fake = server([
      plugin({
        id: 1,
        state: "approved",
        settings: [
          { key: "apiKey", label: "API key", secret: true, required: true, set: false },
        ],
      }),
    ]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    const row = await waitFor(() => rowOf("example.works"));
    expect(within(row).getByText("Needs a value: API key")).toBeDefined();
    await user.click(within(row).getByRole("button", { name: "Settings" }));
    let dialog = screen.getByRole("dialog", { name: "Settings for example.works" });
    expect(within(dialog).getByText("Not set.")).toBeDefined();
    await user.type(within(dialog).getByLabelText(/API key/), "s3cret-value");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await screen.findByText("Saved the settings for example.works."),
    ).toBeDefined();
    expect(fake.requests).toContainEqual({
      method: "PUT",
      url: "/api/work-info-plugins/1/settings",
      body: { values: { apiKey: "s3cret-value" } },
    });
    const saved = rowOf("example.works");
    expect(within(saved).getByText(/Settings: API key set/)).toBeDefined();
    expect(within(saved).queryByText(/Needs a value/)).toBeNull();
    expect(within(saved).getByText("Approved")).toBeDefined();
    expect(document.body.textContent).not.toContain("s3cret-value");

    await user.click(within(saved).getByRole("button", { name: "Settings" }));
    dialog = screen.getByRole("dialog", { name: "Settings for example.works" });
    const input = within(dialog).getByLabelText<HTMLInputElement>(/API key/);
    expect(input.value).toBe("");
    expect(input.type).toBe("password");
    expect(
      within(dialog).getByText("Saved. Type a new value to replace it."),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Forget" }));
    expect(within(dialog).getByText("Removed when you save.")).toBeDefined();
    expect(input.disabled).toBe(true);
    await user.click(within(dialog).getByRole("button", { name: "Undo" }));
    expect(input.disabled).toBe(false);
    expect(
      within(dialog).getByRole("button", { name: "Save" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("既定値は入力した値と見分けて示し、選択肢は既定値を選んだ形で出し、選んだ値と消した値を送る", async () => {
    const fake = server([
      plugin({
        id: 1,
        state: "approved",
        settings: [
          {
            key: "region",
            label: "Region",
            secret: false,
            required: true,
            set: false,
            default: "jp",
          },
          {
            key: "sort",
            label: "Sort",
            secret: false,
            required: false,
            set: false,
            default: "new",
            options: ["new", "popular"],
          },
          {
            key: "lang",
            label: "Language",
            secret: false,
            required: false,
            set: true,
            value: "en",
            default: "ja",
          },
        ],
      }),
    ]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    const row = await waitFor(() => rowOf("example.works"));
    expect(
      within(row).getByText(/Settings: Region default, Sort default, Language set/),
    ).toBeDefined();
    expect(within(row).queryByText(/Needs a value/)).toBeNull();
    await user.click(within(row).getByRole("button", { name: "Settings" }));
    const dialog = screen.getByRole("dialog", { name: "Settings for example.works" });

    // 既定値は欄の値ではなく placeholder と欄の下の注記に出る。
    const region = within(dialog).getByLabelText<HTMLInputElement>(/Region/);
    expect(region.value).toBe("");
    expect(region.placeholder).toBe("jp");
    expect(within(dialog).getByText("Default: jp")).toBeDefined();
    const lang = within(dialog).getByLabelText<HTMLInputElement>(/Language/);
    expect(lang.value).toBe("en");
    expect(within(dialog).getByText("Default: ja")).toBeDefined();

    // 選択肢の設定は Select だけで、既定値の項目が選ばれている。
    expect(within(dialog).queryByRole("textbox", { name: /Sort/ })).toBeNull();
    const sort = within(dialog).getByRole("combobox", { name: /Sort/ });
    expect(sort.textContent).toBe("Default (new)");
    await user.click(sort);
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual([
      "Default (new)",
      "new",
      "popular",
    ]);
    await user.click(screen.getByRole("option", { name: "popular" }));
    expect(sort.textContent).toBe("popular");

    await user.clear(lang);
    expect(lang.placeholder).toBe("ja");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await screen.findByText("Saved the settings for example.works."),
    ).toBeDefined();
    expect(fake.requests).toContainEqual({
      method: "PUT",
      url: "/api/work-info-plugins/1/settings",
      body: { values: { sort: "popular", lang: null } },
    });
    expect(
      within(rowOf("example.works")).getByText(
        /Settings: Region default, Sort set, Language default/,
      ),
    ).toBeDefined();
  });

  it("選択肢に無い保存値は注記して既定値の形で出し、選び直すとその値を送る", async () => {
    const fake = server([
      plugin({
        id: 1,
        settings: [
          {
            key: "mode",
            label: "Mode",
            secret: false,
            required: false,
            set: true,
            value: "old",
            staleValue: "old",
            default: "fast",
            options: ["fast", "slow"],
          },
        ],
      }),
    ]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    const row = await waitFor(() => rowOf("example.works"));
    expect(within(row).getByText(/Settings: Mode not set/)).toBeDefined();
    expect(within(row).getByText("Choose again: Mode")).toBeDefined();

    // 承認のプロンプトは既定値を設定の行の末尾に出す。
    await user.click(within(row).getByRole("button", { name: "Review and approve" }));
    const approval = screen.getByRole("dialog", { name: "Approve example.works?" });
    expect(within(approval).getByText("Default: fast")).toBeDefined();
    await user.keyboard("{Escape}");

    await user.click(within(row).getByRole("button", { name: "Settings" }));
    const dialog = screen.getByRole("dialog", { name: "Settings for example.works" });
    expect(
      within(dialog).getByText(
        `"old" isn't one of this version's choices. The default is used until you save.`,
      ),
    ).toBeDefined();
    const mode = within(dialog).getByRole("combobox", { name: /Mode/ });
    expect(mode.textContent).toBe("Default (fast)");
    expect(
      within(dialog).getByRole("button", { name: "Save" }).hasAttribute("disabled"),
    ).toBe(true);
    await user.click(mode);
    await user.click(await screen.findByRole("option", { name: "slow" }));
    expect(within(dialog).queryByText(/isn't one of this version's choices/)).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await screen.findByText("Saved the settings for example.works."),
    ).toBeDefined();
    expect(fake.requests).toContainEqual({
      method: "PUT",
      url: "/api/work-info-plugins/1/settings",
      body: { values: { mode: "slow" } },
    });
    const saved = rowOf("example.works");
    expect(within(saved).getByText(/Settings: Mode set/)).toBeDefined();
    expect(within(saved).queryByText(/Choose again/)).toBeNull();
  });

  it("失敗した要求を節に出す", async () => {
    let failLoad = true;
    const fake = server([plugin({ id: 1, state: "approved" })], (method, url) => {
      if (method === "GET" && failLoad)
        return json({ code: "internal_error", message: "boom" }, 500);
      if (method === "DELETE" && url.endsWith("/approval"))
        return json({ code: "not_found", message: "not found" }, 404);
      if (method === "POST")
        return json({ code: "plugin_too_large", message: "too large", limit: 1 }, 413);
      return undefined;
    });
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    expect(
      await screen.findByText(/^Couldn't load the work-info plugins:/),
    ).toBeDefined();
    expect(
      screen.getByRole("button", { name: "Add plugin" }).getAttribute("aria-disabled"),
    ).toBe("true");
    failLoad = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    const row = await waitFor(() => rowOf("example.works"));
    await user.click(
      within(row).getByRole("button", { name: "More actions for example.works" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Revoke approval" }));
    expect(await within(row).findByText(/^Couldn't change example.works:/)).toBeDefined();

    choose(wasm("example.wasm"));
    expect(
      await screen.findByText("Couldn't add the plugin: The file is over 16 MB."),
    ).toBeDefined();
  });

  it("16 MiB を超えるファイルは送る前に断る", async () => {
    const fake = server([]);
    fetchMock.mockImplementation(fake.handler);
    renderSection();
    await screen.findByText("No plugins yet");

    choose(wasm("example.wasm", 16 * 1024 * 1024 + 1));

    expect(await screen.findByText("Choose a file up to 16 MB.")).toBeDefined();
    expect(fake.requests.some((request) => request.method === "POST")).toBe(false);
  });

  it("取り消しは確認なしで未承認に戻し、削除は確認の後に行を除いて次の行へ移る", async () => {
    const fake = server([
      plugin({ id: 1, state: "approved" }),
      plugin({ id: 2, name: "other.site", fileName: "other.wasm" }),
    ]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    const first = await waitFor(() => rowOf("example.works"));
    await user.click(
      within(first).getByRole("button", { name: "More actions for example.works" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Revoke approval" }));
    await waitFor(() => expect(within(first).getByText("Not approved")).toBeDefined());

    await user.click(
      within(first).getByRole("button", { name: "More actions for example.works" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    const confirm = screen.getByRole("alertdialog", { name: "Delete example.works?" });
    await user.click(within(confirm).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Deleted example.works.")).toBeDefined();
    expect(within(list()).queryByText("example.works")).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("other.site")).getByRole("button", { name: "Review and approve" }),
      ),
    );
    expect(fake.stored().map((item) => item.id)).toEqual([2]);
  });

  it("追加の後の読み直しより先に取り消しが終わっても、遅れて届いた一覧で承認済みに戻らない", async () => {
    const fake = server([plugin({ id: 1, state: "approved" })]);
    // 追加の後の一覧は、取り消しの前のサーバーの行を持ったまま、取り消しの後に届く。
    let release: () => void = () => {};
    let gets = 0;
    fetchMock.mockImplementation((input, init) => {
      const method = init?.method ?? "GET";
      if (
        String(input) === "/api/work-info-plugins" &&
        method === "GET" &&
        ++gets === 2
      ) {
        const stale = json({ plugins: fake.stored() });
        return new Promise((resolve) => {
          release = () => resolve(stale);
        });
      }
      return fake.handler(input, init);
    });
    const user = userEvent.setup();
    renderSection();
    const row = await waitFor(() => rowOf("example.works"));

    choose(wasm("broken.wasm"));
    await waitFor(() => expect(gets).toBe(2));
    await user.click(
      within(row).getByRole("button", { name: "More actions for example.works" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Revoke approval" }));
    await waitFor(() => expect(within(row).getByText("Not approved")).toBeDefined());
    release();

    // 追加が終わる（読み直しが済む）まで待つ。
    await screen.findByRole("button", { name: "Add plugin" });
    expect(within(rowOf("example.works")).getByText("Not approved")).toBeDefined();
    expect(within(rowOf("example.works")).queryByText("Approved")).toBeNull();
  });

  it("追加の後の読み直しが失敗しても、足した行を出し一覧を誤りにしない", async () => {
    const fake = server([plugin({ id: 1, name: "other.site", fileName: "other.wasm" })]);
    let gets = 0;
    fetchMock.mockImplementation((input, init) => {
      const method = init?.method ?? "GET";
      if (String(input) === "/api/work-info-plugins" && method === "GET" && ++gets === 2)
        return Promise.resolve(json({ code: "internal_error", message: "boom" }, 500));
      return fake.handler(input, init);
    });
    renderSection();
    await waitFor(() => rowOf("other.site"));

    choose(wasm("example.wasm"));

    expect(
      await screen.findByText("Added example.works. Approve it to use it."),
    ).toBeDefined();
    await waitFor(() => expect(gets).toBe(2));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Add plugin" }).textContent).toBe(
        "Add plugin",
      ),
    );
    expect(within(rowOf("example.works")).getByText("Not approved")).toBeDefined();
    expect(rowOf("other.site")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("保存を送っている間は設定の欄と補助の操作を止める", async () => {
    const fake = server([
      plugin({
        id: 1,
        settings: [
          { key: "region", label: "Region", secret: false, required: false, set: false },
          { key: "apiKey", label: "API key", secret: true, required: false, set: true },
        ],
      }),
    ]);
    let release: () => void = () => {};
    fetchMock.mockImplementation((input, init) => {
      if (init?.method === "PUT" && String(input).endsWith("/settings"))
        return new Promise((resolve) => {
          release = () => void fake.handler(input, init).then(resolve);
        });
      return fake.handler(input, init);
    });
    const user = userEvent.setup();
    renderSection();

    const row = await waitFor(() => rowOf("example.works"));
    await user.click(within(row).getByRole("button", { name: "Settings" }));
    const dialog = screen.getByRole("dialog", { name: "Settings for example.works" });
    await user.type(within(dialog).getByLabelText("Region"), "jp");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(within(dialog).getByLabelText<HTMLInputElement>("Region").disabled).toBe(
        true,
      ),
    );
    expect(within(dialog).getByLabelText<HTMLInputElement>(/API key/).disabled).toBe(
      true,
    );
    expect(
      within(dialog).getByRole<HTMLButtonElement>("button", { name: "Forget" }).disabled,
    ).toBe(true);
    release();
    expect(
      await screen.findByText("Saved the settings for example.works."),
    ).toBeDefined();
  });

  it("設定の値の上限はサーバーと同じく符号位置で数える（絵文字も 2000 個まで入る）", async () => {
    fetchMock.mockImplementation(
      server([
        plugin({
          id: 1,
          settings: [
            {
              key: "region",
              label: "Region",
              secret: false,
              required: false,
              set: false,
            },
          ],
        }),
      ]).handler,
    );
    const user = userEvent.setup();
    renderSection();

    const row = await waitFor(() => rowOf("example.works"));
    await user.click(within(row).getByRole("button", { name: "Settings" }));
    const input = screen.getByLabelText<HTMLInputElement>("Region");
    expect(input.maxLength).toBe(-1);
    fireEvent.change(input, { target: { value: "😀".repeat(2001) } });

    expect(input.value).toBe("😀".repeat(2000));
  });

  it("画面の文言はカタログか利用者のデータだけである", async () => {
    enablePseudoLocale();
    fetchMock.mockImplementation(
      server([
        plugin({
          id: 1,
          state: "approved",
          settings: [
            { key: "apiKey", label: "API key", secret: true, required: true, set: false },
            {
              key: "mode",
              label: "Mode",
              secret: false,
              required: false,
              set: true,
              value: "old",
              staleValue: "old",
              default: "fast",
              options: ["fast", "slow"],
            },
            {
              key: "region",
              label: "Region",
              secret: false,
              required: true,
              set: false,
              default: "jp",
            },
          ],
        }),
        plugin({ ...uploads["broken.wasm"], id: 2 }),
      ]).handler,
    );
    const { container } = renderSection();
    await screen.findByText("example.works");
    expectCatalogTextOnly(container, [
      "example.works",
      "Fetches titles from example.works",
      "broken.wasm",
      "API key",
      "Mode",
      "Region",
      "1.2.0",
    ]);
  });
});
