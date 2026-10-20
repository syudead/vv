import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode, useEffect, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { OwnerAudience } from "../testing/audience";
import { ScanProvider, useScan, useScanControls } from "./ScanProvider";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(id: number, state: Scan["state"], values: Partial<Scan> = {}): Scan {
  return {
    id,
    origin: "manual",
    status: state,
    videos: { total: 1, settled: state === "running" ? 0 : 1 },
    issues: { failed: 0, substituted: 0, revision: 0 },
    state,
    ...values,
  };
}

function Harness() {
  const value = useScan();
  return (
    <>
      <button type="button" onClick={value.start}>
        開始
      </button>
      <button type="button" onClick={value.refresh}>
        更新
      </button>
      <button type="button" onClick={() => value.setFolderCount(1)}>
        フォルダ追加を反映
      </button>
      <p>{value.error ?? "エラーなし"}</p>
      <p>状態: {value.scan?.id ?? "なし"}</p>
      <p>実行中: {value.running ? "はい" : "いいえ"}</p>
      <p>完了: {value.finished?.id ?? "なし"}</p>
      <p>自動の完了: {value.watchFinished?.id ?? "なし"}</p>
      <p>開始可否: {value.canStart ? "可" : "不可"}</p>
      <p>今の処理: {value.activity?.fileName ?? "なし"}</p>
    </>
  );
}

function RescanHarness() {
  const { rescan } = useScanControls();
  const [result, setResult] = useState("なし");
  const [recovered, setRecovered] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={() =>
          void rescan({ scope: "library" }, () => setRecovered(true)).then((failure) =>
            setResult(failure ?? "開始"),
          )
        }
      >
        強制再スキャン
      </button>
      <p>結果: {result}</p>
      <p>回復: {recovered ? "はい" : "いいえ"}</p>
    </>
  );
}

describe("ScanProvider", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
    installFakeEventSource();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("開始要求の失敗を状態取得の成功で消さない", async () => {
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") {
        return Promise.resolve(
          json({ code: "internal", message: "開始できません" }, 500),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    await screen.findByText("エラーなし");

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(
      await screen.findByText(
        "Couldn't start the scan: Something went wrong on the server.",
      ),
    ).toBeDefined();
    const currentCalls = fetchMock.mock.calls.filter(
      ([input]) => String(input) === "/api/scans/current",
    );
    expect(currentCalls.length).toBeGreaterThanOrEqual(2);
  });

  it("開始応答を失っても新しい取り込みの完了を追跡する", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") {
        return Promise.resolve(
          json({ code: "internal", message: "応答を失いました" }, 500),
        );
      }
      currentCalls += 1;
      return Promise.resolve(json(scan(currentCalls === 1 ? 1 : 2, "done")));
    });
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    await waitFor(() => expect(currentCalls).toBe(1));

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(await screen.findByText("完了: 2")).toBeDefined();
    expect(screen.getByText("エラーなし")).toBeDefined();
  });

  it("初回状態取得前に開始して高速完了した取り込みを通知する", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") return Promise.resolve(json(scan(2, "running"), 202));
      currentCalls += 1;
      if (currentCalls === 1) return new Promise<Response>(() => undefined);
      return Promise.resolve(json(scan(2, "done")));
    });
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    await waitFor(() => expect(currentCalls).toBe(1));

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(await screen.findByText("完了: 2")).toBeDefined();
  });

  it("開始の応答より先に届いた完了の知らせを、遅れた応答の実行中で戻さない", async () => {
    let resolveStart: ((response: Response) => void) | undefined;
    let currentCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") {
        return new Promise<Response>((resolve) => {
          resolveStart = resolve;
        });
      }
      currentCalls += 1;
      if (currentCalls === 1) return Promise.resolve(json(scan(4, "done")));
      return Promise.resolve(json({ code: "internal", message: "失敗" }, 500));
    });
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("状態: 4")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "開始" }));
    await waitFor(() => expect(resolveStart).toBeDefined());
    await emitServerEvent("scan", scan(5, "done"));
    expect(screen.getByText("完了: 5")).toBeDefined();

    await act(async () => resolveStart?.(json(scan(5, "running"), 202)));

    await waitFor(() => expect(currentCalls).toBe(2));
    expect(screen.getByText("状態: 5")).toBeDefined();
    expect(screen.getByText("実行中: いいえ")).toBeDefined();
  });

  it("取り込みの完了を変化の知らせで追跡し、一定間隔では問い合わせない", async () => {
    vi.useFakeTimers();
    let currentCalls = 0;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      currentCalls += 1;
      return Promise.resolve(json(scan(3, "running")));
    });
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    await act(async () => Promise.resolve());
    expect(screen.getByText("状態: 3")).toBeDefined();
    const callsAfterLoad = currentCalls;

    await act(async () => vi.advanceTimersByTimeAsync(10000));
    expect(currentCalls).toBe(callsAfterLoad);

    await emitServerEvent("scan", scan(3, "done"));
    expect(screen.getByText("完了: 3")).toBeDefined();
    expect(currentCalls).toBe(callsAfterLoad);
  });

  it("開いた画面の取り直しと最初の接続で、同じ取得を続けて送らない", async () => {
    let currentCalls = 0;
    let folderCalls = 0;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") {
        folderCalls += 1;
        return Promise.resolve(json([{}]));
      }
      currentCalls += 1;
      return Promise.resolve(json(scan(3, "done")));
    });
    // Child list effects request refresh before ScanProvider's own effect runs.
    function Page() {
      const { refresh } = useScanControls();
      useEffect(() => refresh(), [refresh]);
      return null;
    }
    render(
      <OwnerAudience>
        <ScanProvider>
          <Page />
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("状態: 3")).toBeDefined();
    await emitServerEvent("open");
    await emitServerEvent("scan", scan(3, "done"));
    expect(currentCalls).toBe(1);
    expect(folderCalls).toBe(1);

    await emitServerEvent("open");
    await waitFor(() => expect(currentCalls).toBe(2));
  });

  it("StrictMode で effect の後始末が取得を打ち切っても、続く実行で取り直す", async () => {
    // Match fetch: aborted requests reject rather than deliver a response.
    fetchMock.mockImplementation(async (input, init) => {
      await Promise.resolve();
      if (init?.signal?.aborted) throw new DOMException("aborted", "AbortError");
      if (String(input) === "/api/media-folders") return json([{}]);
      return json(scan(3, "done"));
    });
    render(
      <StrictMode>
        <OwnerAudience>
          <ScanProvider>
            <Harness />
          </ScanProvider>
        </OwnerAudience>
      </StrictMode>,
    );
    expect(await screen.findByText("状態: 3")).toBeDefined();
    expect(screen.getByText("開始可否: 可")).toBeDefined();
  });

  it("返らないまま残った取得は、少し経ってからの取り直しで打ち切って取り直す", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      currentCalls += 1;
      if (currentCalls === 1) {
        return new Promise((_, reject) => {
          init?.signal?.addEventListener("abort", () =>
            reject(new DOMException("aborted", "AbortError")),
          );
        });
      }
      return Promise.resolve(json(scan(5, "done")));
    });
    const now = vi.spyOn(Date, "now").mockReturnValue(1_000_000);
    try {
      render(
        <OwnerAudience>
          <ScanProvider>
            <Harness />
          </ScanProvider>
        </OwnerAudience>,
      );
      await act(async () => Promise.resolve());
      await userEvent.click(screen.getByRole("button", { name: "更新" }));
      expect(currentCalls).toBe(1);

      now.mockReturnValue(1_000_000 + 5_000);
      await userEvent.click(screen.getByRole("button", { name: "更新" }));
      expect(await screen.findByText("状態: 5")).toBeDefined();
      expect(currentCalls).toBe(2);
    } finally {
      now.mockRestore();
    }
  });

  it("取り込みの進みの知らせでは、useScanControls を読む部品を描き直さない", async () => {
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      return Promise.resolve(json(scan(4, "running")));
    });
    let renders = 0;
    function Controls() {
      const { running } = useScanControls();
      renders += 1;
      return <p>操作の実行中: {running ? "はい" : "いいえ"}</p>;
    }
    render(
      <OwnerAudience>
        <ScanProvider>
          <Controls />
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("操作の実行中: はい")).toBeDefined();
    const before = renders;
    for (let settled = 1; settled <= 5; settled++) {
      await emitServerEvent(
        "scan",
        scan(4, "running", {
          videos: { total: 10, settled },
          activity: { kind: "probe", fileName: `v${String(settled)}.mp4` },
        }),
      );
    }
    expect(screen.getByText("今の処理: v5.mp4")).toBeDefined();
    expect(renders).toBe(before);

    await emitServerEvent("scan", scan(4, "done"));
    expect(await screen.findByText("操作の実行中: いいえ")).toBeDefined();
  });

  it("取り込み中に今の処理が一瞬無くなっても、直前の値を残す", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") throw new Error("/api/processing は呼ばない");
      return Promise.resolve(json(scan(1, "running")));
    });
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("今の処理: なし")).toBeDefined();

    const activity = { kind: "probe", fileName: "a.mp4" } as const;
    await emitServerEvent("scan", scan(1, "done", { status: "running", activity }));
    expect(screen.getByText("今の処理: a.mp4")).toBeDefined();

    await emitServerEvent("scan", scan(1, "done", { status: "running" }));
    expect(screen.getByText("今の処理: a.mp4")).toBeDefined();

    await emitServerEvent("scan", scan(1, "done"));
    expect(screen.getByText("今の処理: なし")).toBeDefined();

    await emitServerEvent("scan", scan(2, "running", { activity }));
    await emitServerEvent("scan", scan(3, "running"));
    expect(screen.getByText("今の処理: なし")).toBeDefined();
    expect(
      fetchMock.mock.calls.some(([input]) => String(input) === "/api/processing"),
    ).toBe(false);
  });

  it("初回の状態取得に失敗しても、知らせの接続がつながったら取り直す", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      currentCalls += 1;
      if (currentCalls === 1) return Promise.reject(new Error("一時的な失敗"));
      return Promise.resolve(json(scan(4, "done")));
    });
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(
      await screen.findByText(
        "Couldn't reach the server. Check that VVMDM is running and try again.",
      ),
    ).toBeDefined();
    expect(screen.getByText("状態: なし")).toBeDefined();

    await emitServerEvent("open");

    expect(await screen.findByText("状態: 4")).toBeDefined();
    expect(screen.getByText("エラーなし")).toBeDefined();
  });

  it("状態取得の一時失敗中も最後の成功状態を保持する", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      currentCalls += 1;
      if (currentCalls === 1) return Promise.resolve(json(scan(5, "running")));
      return Promise.reject(new Error("一時的な失敗"));
    });
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("状態: 5")).toBeDefined();

    await act(async () => window.dispatchEvent(new Event("focus")));

    expect(
      await screen.findByText(
        "Couldn't reach the server. Check that VVMDM is running and try again.",
      ),
    ).toBeDefined();
    expect(screen.getByText("状態: 5")).toBeDefined();
  });

  it("再確認で別タブが完了した取り込みを通知する", async () => {
    let currentId = 1;
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan(currentId, "done")),
      ),
    );
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    await screen.findByText("完了: なし");

    currentId = 2;
    await user.click(screen.getByRole("button", { name: "更新" }));

    expect(await screen.findByText("完了: 2")).toBeDefined();
  });

  it("フォルダ0件では設定画面を開かなくても開始要求を送らない", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([]) : json({}, 404)),
    );
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );

    expect(await screen.findByText("開始可否: 不可")).toBeDefined();
    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
  });

  it("windowへ戻ったとき別タブで削除された最後のフォルダを反映する", async () => {
    let folders = [{}];
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json(folders) : json({}, 404),
      ),
    );
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("開始可否: 可")).toBeDefined();

    folders = [];
    window.dispatchEvent(new Event("focus"));

    expect(await screen.findByText("開始可否: 不可")).toBeDefined();
  });

  it("開始時にフォルダ未設定なら開始可否も不可へ同期する", async () => {
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") {
        return Promise.resolve(
          json(
            {
              code: "media_folders_not_configured",
              message: "No media folders yet",
            },
            409,
          ),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("開始可否: 可")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(await screen.findByText("開始可否: 不可")).toBeDefined();
  });

  it("起動時の遅い応答で後発のフォルダ件数を上書きしない", async () => {
    let resolveFolders: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") {
        return new Promise<Response>((resolve) => {
          resolveFolders = resolve;
        });
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );

    await user.click(screen.getByRole("button", { name: "フォルダ追加を反映" }));
    expect(screen.getByText("開始可否: 可")).toBeDefined();
    resolveFolders?.(json([]));

    expect(await screen.findByText("開始可否: 可")).toBeDefined();
  });

  it("取得の途中で届いた知らせを、遅れて返った古い応答で上書きしない", async () => {
    let resolveCurrent: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      return new Promise<Response>((resolve) => {
        resolveCurrent = resolve;
      });
    });
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    await waitFor(() => expect(resolveCurrent).toBeDefined());

    await emitServerEvent("scan", scan(7, "done"));
    expect(screen.getByText("状態: 7")).toBeDefined();

    // Deliver an older running snapshot after a newer notification.
    await act(async () => resolveCurrent?.(json(scan(6, "running"))));

    expect(screen.getByText("状態: 7")).toBeDefined();
  });

  describe("強制再スキャン", () => {
    function serveRescan(
      post: () => Response,
      current: (posted: boolean) => Scan,
      folders: unknown[] = [{}],
    ) {
      let posted = false;
      fetchMock.mockImplementation((input, init) => {
        if (String(input) === "/api/media-folders") return Promise.resolve(json(folders));
        if (init?.method === "POST") {
          posted = true;
          return Promise.resolve(post());
        }
        return Promise.resolve(json(current(posted)));
      });
    }

    function renderRescan() {
      render(
        <OwnerAudience>
          <ScanProvider>
            <RescanHarness />
            <Harness />
          </ScanProvider>
        </OwnerAudience>,
      );
    }

    const posts = () =>
      fetchMock.mock.calls.filter(([, init]) => init?.method === "POST");

    it("フォルダ0件でも開始要求を送り、断られた理由を返す", async () => {
      serveRescan(
        () => json({ code: "media_folders_not_configured", message: "none" }, 409),
        () => scan(1, "done"),
        [],
      );
      renderRescan();
      const user = userEvent.setup();
      expect(await screen.findByText("開始可否: 不可")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "強制再スキャン" }));

      expect(await screen.findByText("結果: Add a media folder first.")).toBeDefined();
      expect(posts().map(([input]) => String(input))).toEqual(["/api/scans/rescan"]);
    });

    it("応答を失った開始は、後で見つかった新しい取り込みで呼び出し元へ回復を知らせる", async () => {
      serveRescan(
        () => json({ code: "internal", message: "lost" }, 500),
        (posted) => (posted ? scan(2, "running") : scan(1, "done")),
      );
      renderRescan();
      const user = userEvent.setup();
      expect(await screen.findByText("状態: 1")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "強制再スキャン" }));

      expect(
        await screen.findByText("結果: Something went wrong on the server."),
      ).toBeDefined();
      expect(await screen.findByText("回復: はい")).toBeDefined();
    });

    it("409 で断られたときは、実行中の取り込みが見えても回復にしない", async () => {
      serveRescan(
        () => json({ code: "scan_in_progress", message: "busy" }, 409),
        (posted) => (posted ? scan(2, "running") : scan(1, "done")),
      );
      renderRescan();
      const user = userEvent.setup();
      expect(await screen.findByText("状態: 1")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "強制再スキャン" }));

      expect(await screen.findByText("状態: 2")).toBeDefined();
      expect(screen.getByText(/^結果: A scan is in progress/)).toBeDefined();
      expect(screen.getByText("回復: いいえ")).toBeDefined();
    });
  });

  describe("自動の取り込み（origin watch）", () => {
    function mount() {
      fetchMock.mockImplementation((input) => {
        if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
        return Promise.resolve(json({}, 404));
      });
      return render(
        <OwnerAudience>
          <ScanProvider>
            <Harness />
          </ScanProvider>
        </OwnerAudience>,
      );
    }
    const watch = (id: number, state: Scan["state"], values: Partial<Scan> = {}) =>
      scan(id, state, { origin: "watch", ...values });

    it("走っているあいだは実行中にせず、終わりは手動の完了と別に知らせる", async () => {
      mount();
      expect(await screen.findByText("実行中: いいえ")).toBeDefined();

      await emitServerEvent("scan", watch(30, "running"));
      expect(screen.getByText("状態: 30")).toBeDefined();
      expect(screen.getByText("実行中: いいえ")).toBeDefined();
      expect(screen.getByText("自動の完了: なし")).toBeDefined();

      await emitServerEvent("scan", watch(30, "done"));
      expect(screen.getByText("自動の完了: 30")).toBeDefined();
      expect(screen.getByText("完了: なし")).toBeDefined();
    });

    it("見た時点で終わっていた取り込みは完了にしないが、あとから現れた終わりは完了にする", async () => {
      fetchMock.mockImplementation((input) => {
        if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
        return Promise.resolve(json(watch(40, "done")));
      });
      render(
        <OwnerAudience>
          <ScanProvider>
            <Harness />
          </ScanProvider>
        </OwnerAudience>,
      );
      expect(await screen.findByText("状態: 40")).toBeDefined();
      expect(screen.getByText("自動の完了: なし")).toBeDefined();

      await emitServerEvent("scan", watch(41, "done", { status: "partial" }));
      expect(screen.getByText("自動の完了: 41")).toBeDefined();

      await emitServerEvent(
        "scan",
        watch(41, "done", {
          status: "partial",
          issues: { failed: 1, substituted: 0, revision: 2 },
        }),
      );
      expect(screen.getByText("自動の完了: 41")).toBeDefined();
    });

    it("準備が残るあいだ（state done, status running）は終わりにしない", async () => {
      mount();
      expect(await screen.findByText("実行中: いいえ")).toBeDefined();
      await emitServerEvent("scan", watch(50, "running"));
      await emitServerEvent("scan", watch(50, "done", { status: "running" }));
      expect(screen.getByText("自動の完了: なし")).toBeDefined();
      await emitServerEvent("scan", watch(50, "done", { status: "done" }));
      expect(screen.getByText("自動の完了: 50")).toBeDefined();
    });
  });
});
