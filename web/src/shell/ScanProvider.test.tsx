import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { OwnerAudience } from "../testing/audience";
import { ScanProvider, useScan } from "./ScanProvider";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(id: number, state: Scan["state"]): Scan {
  return { id, state, total: 1, completed: state === "running" ? 0 : 1, failed: 0 };
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
      <p>開始可否: {value.canStart ? "可" : "不可"}</p>
      <p>
        残り:{" "}
        {value.processing === null
          ? "未取得"
          : `${String(value.processing.probe)}/${String(value.processing.thumbnail)}/${String(value.processing.preview)}`}
      </p>
    </>
  );
}

describe("ScanProvider", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    fetchMock.mockReset();
    // 段階ごとの残りはどの検査でも同じ応答にし、scan の問い合わせの数え方に
    // 混ぜない。
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) =>
      String(input) === "/api/processing"
        ? Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }))
        : fetchMock(input, init),
    );
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
      await screen.findByText("取り込みを始められません: 開始できません"),
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
      // 初回は前回の取り込みを返し、開始後の取り直しは一時的に失敗する。
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

  it("段階ごとの残りを変化の知らせで更新する", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan(1, "done")),
      ),
    );
    render(
      <OwnerAudience>
        <ScanProvider>
          <Harness />
        </ScanProvider>
      </OwnerAudience>,
    );
    expect(await screen.findByText("残り: 0/0/0")).toBeDefined();

    await emitServerEvent("processing", { probe: 3, thumbnail: 2, preview: 1 });

    expect(screen.getByText("残り: 3/2/1")).toBeDefined();
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
    expect(await screen.findByText("一時的な失敗")).toBeDefined();
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

    expect(await screen.findByText("一時的な失敗")).toBeDefined();
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
              message: "メディアフォルダが設定されていません",
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

    await waitFor(() => expect(screen.getByText("開始可否: 可")).toBeDefined());
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

    // 知らせより前に読んだ実行中の状態が、あとから返る。
    await act(async () => resolveCurrent?.(json(scan(6, "running"))));

    expect(screen.getByText("状態: 7")).toBeDefined();
  });
});
