import { useEffect, useState } from "react";

import type { components } from "./api/gen/openapi";

// 型は api/openapi.yaml からの生成物を使う。契約を変えると、ここが
// コンパイルエラーになって気付ける（research.md R-010）。
type Health = components["schemas"]["Health"];

type State =
  | { kind: "loading" }
  | { kind: "ready"; health: Health }
  | { kind: "failed"; reason: string };

// Phase 0 の画面は稼働状態の表示1つだけなので、ルーターとサーバー状態の
// キャッシュは入れない（research.md R-011）。
function useHealth(): State {
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    const controller = new AbortController();

    void (async () => {
      try {
        const response = await fetch("/api/health", { signal: controller.signal });
        // 503（degraded）でも本文は Health なので、そのまま表示する。
        const health = (await response.json()) as Health;
        setState({ kind: "ready", health });
      } catch (error) {
        if (controller.signal.aborted) {
          return;
        }
        setState({
          kind: "failed",
          reason: error instanceof Error ? error.message : String(error),
        });
      }
    })();

    return () => controller.abort();
  }, []);

  return state;
}

const statusStyles: Record<Health["status"], string> = {
  ok: "bg-emerald-100 text-emerald-900",
  degraded: "bg-amber-100 text-amber-900",
};

export default function App() {
  const state = useHealth();

  return (
    <main className="mx-auto flex min-h-dvh max-w-xl flex-col justify-center gap-6 p-8">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight">vv</h1>
        <p className="text-sm text-neutral-600">セルフホストの動画管理システム</p>
      </header>

      <section className="rounded-lg border border-neutral-200 p-4">
        <h2 className="mb-3 text-sm font-medium text-neutral-500">稼働状態</h2>

        {state.kind === "loading" && <p className="text-neutral-600">確認中…</p>}

        {state.kind === "failed" && (
          <p className="text-red-700">/api/health に到達できません: {state.reason}</p>
        )}

        {state.kind === "ready" && (
          <dl className="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-2">
            <dt className="text-sm text-neutral-500">status</dt>
            <dd>
              <span
                className={`rounded px-2 py-0.5 font-mono text-sm ${statusStyles[state.health.status]}`}
              >
                {state.health.status}
              </span>
            </dd>

            <dt className="text-sm text-neutral-500">version</dt>
            <dd className="font-mono text-sm">{state.health.version}</dd>

            {state.health.commit !== undefined && (
              <>
                <dt className="text-sm text-neutral-500">commit</dt>
                <dd className="truncate font-mono text-sm">{state.health.commit}</dd>
              </>
            )}
          </dl>
        )}
      </section>
    </main>
  );
}
