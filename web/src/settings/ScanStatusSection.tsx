import { AlertTriangle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

import Button from "../ui/Button";
import ScanProgressBar from "../shell/ScanProgressBar";
import { useScan } from "../shell/ScanProvider";
import { presentScan, type ScanPresentation } from "../shell/scanPresentation";

function formatTime(value?: string) {
  if (value === undefined) return "未完了";
  return new Intl.DateTimeFormat("ja-JP", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function stateLabel(presentation: ScanPresentation) {
  switch (presentation.state) {
    case "not-run":
      return "未実行";
    case "starting":
      return "開始中";
    case "unknown-total":
      return "実行中";
    case "running":
      return "実行中";
    case "done":
      return "完了";
    case "partial-failed":
      return "一部失敗";
    case "failed":
      return "失敗";
    case "fetch-failed":
      return "再確認中";
  }
}

function StateIcon({ state }: { state: ScanPresentation["state"] }) {
  if (state === "done") return <CheckCircle2 className="size-4" />;
  if (state === "partial-failed") return <AlertTriangle className="size-4" />;
  if (state === "failed") return <XCircle className="size-4" />;
  return <RefreshCw className="size-4" />;
}

export default function ScanStatusSection() {
  const location = useLocation();
  const heading = useRef<HTMLHeadingElement>(null);
  const scan = useScan();
  const presentation = presentScan(scan);
  const retry = () => scan.start();
  const empty = presentation.state === "not-run";
  const noItems = presentation.state === "done" && presentation.total === 0;

  useLayoutEffect(() => {
    if (location.hash !== "#scan-status") return;
    const focusHeading = () => {
      heading.current?.scrollIntoView?.({ block: "start" });
      heading.current?.focus();
    };
    focusHeading();
    const raf = window.requestAnimationFrame(focusHeading);
    const timers = [
      window.setTimeout(focusHeading, 0),
      window.setTimeout(focusHeading, 100),
    ];
    return () => {
      window.cancelAnimationFrame(raf);
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [location.hash, location.key]);

  return (
    <section id="scan-status" aria-labelledby="scan-status-heading" className="mt-8">
      <div className="border-b border-border pb-4">
        <div className="flex flex-wrap items-center gap-2">
          <h2
            ref={heading}
            id="scan-status-heading"
            tabIndex={-1}
            className="text-base font-semibold"
          >
            取り込み状況
          </h2>
          <span className="inline-flex items-center gap-1 rounded-md bg-elevated px-2 py-1 text-xs text-fg-muted">
            <StateIcon state={presentation.state} />
            {stateLabel(presentation)}
          </span>
        </div>
        <p className="mt-3 text-sm leading-6 text-fg-muted">
          {empty
            ? "まだ取り込んでいません"
            : noItems
              ? "対象はありませんでした"
              : presentation.description}
        </p>
        <div className="mt-4 max-w-xl">
          <ScanProgressBar presentation={presentation} className="h-2" />
        </div>
        <dl className="mt-4 grid grid-cols-1 gap-2 text-sm text-fg-muted sm:grid-cols-3">
          <div>
            <dt>処理済み</dt>
            <dd className="tabular-nums text-fg">{String(presentation.completed)} 件</dd>
          </div>
          <div>
            <dt>総件数</dt>
            <dd className="tabular-nums text-fg">
              {presentation.total === null
                ? "確認中"
                : `${String(presentation.total)} 件`}
            </dd>
          </div>
          <div>
            <dt>失敗</dt>
            <dd className="tabular-nums text-fg">{String(presentation.failed)} 件</dd>
          </div>
        </dl>
        <dl className="mt-4 grid grid-cols-1 gap-2 text-sm text-fg-muted sm:grid-cols-2">
          <div>
            <dt>開始時刻</dt>
            <dd className="text-fg">{formatTime(presentation.startedAt)}</dd>
          </div>
          <div>
            <dt>完了時刻</dt>
            <dd className="text-fg">{formatTime(presentation.finishedAt)}</dd>
          </div>
        </dl>
        {presentation.refreshing && (
          <p role="status" className="mt-3 text-sm text-warning">
            最新状態を再確認中
          </p>
        )}
        {presentation.state === "failed" && (
          <div className="mt-4 flex flex-col items-start gap-3">
            <p className="break-words text-sm text-danger">
              {presentation.scan?.error ??
                presentation.error ??
                "理由は記録されていません"}
            </p>
            <Button
              variant="primary"
              onClick={retry}
              disabled={!scan.canStart || scan.running}
            >
              <RefreshCw />
              再試行
            </Button>
          </div>
        )}
      </div>
    </section>
  );
}
