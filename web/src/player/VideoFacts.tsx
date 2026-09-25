import {
  AlertCircle,
  CalendarPlus,
  Clock,
  Copy,
  ExternalLink,
  HardDrive,
  type LucideIcon,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { openVideoFile, RequestFailed, type Video } from "../api/client";
import { cn } from "../lib/cn";
import { formatBytes, formatDuration } from "../lib/format";
import IconButton from "../ui/IconButton";
import { useToast } from "../ui/Toast";
import { formatDate, technicalSummary } from "./properties";

/**
 * useOpenFile はサーバーの PC で動画ファイルを開き、開けなかったときの文言を持つ。
 * 文言は、次に開く操作をしたとき、または別の動画へ移ったときに消える。
 */
export function useOpenFile(videoId: number): {
  open: () => void;
  failure: string | null;
} {
  const [failure, setFailure] = useState<string | null>(null);
  const request = useRef<AbortController | null>(null);

  useEffect(() => {
    setFailure(null);
    return () => request.current?.abort();
  }, [videoId]);

  const open = useCallback(() => {
    setFailure(null);
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    void openVideoFile(videoId, controller.signal).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setFailure(
        error instanceof RequestFailed && error.code === "file_missing"
          ? "開けませんでした: ファイルが見つかりません"
          : "開けませんでした",
      );
    });
  }, [videoId]);

  return { open, failure };
}

/**
 * copyWithSelection は見えない入力欄に文字を置いて選び、コピーの命令で写す。
 * 選ぶと入力欄にフォーカスが移るので、終わったら元の要素（「パスをコピー」）へ戻す。
 */
function copyWithSelection(text: string): boolean {
  const previous = document.activeElement;
  const field = document.createElement("textarea");
  field.value = text;
  field.setAttribute("readonly", "");
  field.style.position = "fixed";
  field.style.opacity = "0";
  document.body.appendChild(field);
  field.select();
  try {
    // 代わりの無い古い API だが、安全でない接続ではこれしか使えない。
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    field.remove();
    if (previous instanceof HTMLElement && previous.isConnected) previous.focus();
  }
}

function Fact({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
}) {
  return (
    <li title={label} className="flex items-center gap-1.5 whitespace-nowrap">
      <Icon className="size-4 shrink-0 text-fg-subtle" aria-hidden="true" />
      <span className="sr-only">{label} </span>
      {value}
    </li>
  );
}

/**
 * VideoFacts は題名とタグの下の 2 行である（ui-design「Video facts」）。
 *
 * 1 行目はファイルの情報（長さ・サイズ・追加日）をアイコンを添えて並べ、右端に
 * 「ファイルを開く」（開ける環境のときだけ）と「パスをコピー」を置く。2 行目は技術情報
 * （解像度・コンテナ・コーデック）で、いちばん小さく薄い文字にする。
 *
 * 開けなかったときは、1 行目のすぐ下に 1 行だけ出す。帯やトーストは使わない。
 */
export default function VideoFacts({ video }: { video: Video }) {
  const { open, failure } = useOpenFile(video.id);
  const toast = useToast();
  const location = video.location;
  const duration = formatDuration(video.durationMs);
  const technical = technicalSummary(video);

  const copyPath = () => {
    if (location === undefined) return;
    const path = location.path;
    const done = () => toast("パスをコピーしました");
    const fallback = () =>
      copyWithSelection(path) ? done() : toast("パスをコピーできませんでした");
    // navigator.clipboard は安全な接続（HTTPS・localhost）でしか使えない。LAN のアドレスで
    // 開いたときは、選んだ文字をコピーする古い方法に切り替える。
    if (navigator.clipboard === undefined) {
      fallback();
      return;
    }
    void navigator.clipboard.writeText(path).then(done, fallback);
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <ul
          aria-label="ファイルの情報"
          className="flex min-w-0 flex-1 flex-wrap items-center gap-x-4 sm:gap-x-5 gap-y-2 text-sm text-fg tabular-nums"
        >
          {duration !== "" && <Fact icon={Clock} label="長さ" value={duration} />}
          <Fact icon={HardDrive} label="サイズ" value={formatBytes(video.sizeBytes)} />
          <Fact icon={CalendarPlus} label="追加日" value={formatDate(video.addedAt)} />
        </ul>
        {location !== undefined && (
          <div className="ml-auto flex shrink-0 items-center">
            {location.openable && (
              <IconButton
                label="ファイルを開く"
                size="sm"
                onClick={open}
                className="text-fg-muted! hover:text-fg!"
              >
                <ExternalLink aria-hidden="true" />
              </IconButton>
            )}
            <IconButton
              label="パスをコピー"
              size="sm"
              onClick={copyPath}
              className="text-fg-muted! hover:text-fg!"
            >
              <Copy aria-hidden="true" />
            </IconButton>
          </div>
        )}
      </div>
      {failure !== null && (
        <p role="alert" className="-mt-1 flex items-center gap-2 text-sm text-danger">
          <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
          {failure}
        </p>
      )}
      <TechnicalLine summary={technical} />
    </div>
  );
}

function TechnicalLine({ summary }: { summary: ReturnType<typeof technicalSummary> }) {
  const base = "text-xs tracking-wider tabular-nums";
  if (summary.kind === "pending") {
    return <p className={cn(base, "text-fg-muted")}>技術情報を読み取り中</p>;
  }
  if (summary.kind === "failed") {
    return <p className={cn(base, "text-warning")}>技術情報を読み取れませんでした</p>;
  }
  if (summary.values.length === 0) return null;
  // どの項目も左に縦線と余白を持ち、並び全体をその幅だけ左へずらして外側で切る。
  // 折り返した行の先頭の項目も、線と余白が切り落とされて行頭に残らない。
  return (
    <div className="overflow-hidden">
      <ul
        aria-label="技術情報"
        lang="en"
        className={cn(
          base,
          "-ml-[calc(0.625rem+1px)] flex flex-wrap items-center gap-y-1 text-fg-muted uppercase",
        )}
      >
        {summary.values.map((value) => (
          <li key={value} className="border-l border-border-strong px-2.5 leading-none">
            {value}
          </li>
        ))}
      </ul>
    </div>
  );
}
