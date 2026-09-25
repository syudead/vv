import {
  AlertCircle,
  AlertTriangle,
  Circle,
  CircleCheck,
  FolderOpen,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
} from "lucide-react";
import { useState, type ReactNode } from "react";

import type { Video } from "../api/client";
import { cn } from "../lib/cn";
import { formatDuration } from "../lib/format";
import Button from "../ui/Button";
import { useOpenFile } from "./VideoFacts";
import { creatingLine, processingStages, type StageState } from "./processing";

/**
 * プレイヤーの中に出す状態表示（要件 10〜13、ui-design「Overlay layer」）。
 *
 * どれもプレイヤーの領域の中に収める。層の中の文字は、半透明の `bg-overlay` の上に直接
 * 置かず、不透明な `bg-navbar` の面に載せる。
 */

const spin = "animate-spin motion-reduce:animate-none";
const fadeIn = "animate-fade-in motion-reduce:animate-none";

/** Panel はプレイヤーの領域の中央に置く、不透明な面である。 */
function Panel({
  children,
  className,
  role,
}: {
  children: ReactNode;
  className?: string;
  role?: "alert" | "status";
}) {
  return (
    <div
      role={role}
      className={cn(
        "pointer-events-auto flex w-full flex-col rounded-lg bg-navbar p-5",
        className,
      )}
    >
      {children}
    </div>
  );
}

/**
 * Surface は、プレイヤーが無いときの状態表示の入れ物である。内容が 16:9 に収まらない幅では
 * 領域の高さを内容に合わせて伸ばす（16:9 は下限として入れ物の外で保つ）。
 * `lg` 未満では右上に × が重なるので、上に × の分の余白を取る。
 */
function Surface({ children }: { children: ReactNode }) {
  return (
    <div
      className={cn(
        "flex w-full items-center justify-center bg-navbar p-4 pt-16 sm:px-6 sm:pb-6 lg:p-6",
        fadeIn,
      )}
    >
      {children}
    </div>
  );
}

/**
 * Dimmed は映像を暗くして、その中央に面を置く（再生失敗・再生終了）。操作バーはこの層の
 * 上に出たままなので、下にはその分の余白を取り、面が操作バーの下に潜らないようにする。
 */
export function Dimmed({ children }: { children: ReactNode }) {
  return (
    <div
      className={cn(
        "pointer-events-auto flex w-full items-center justify-center bg-overlay px-4 pt-16 pb-14 sm:px-6 lg:pt-6",
        fadeIn,
      )}
    >
      {children}
    </div>
  );
}

/** LoadingOverlay は読み込み中の輪である。backdrop が偽なら背景（サムネイル）を透かす。 */
export function LoadingOverlay({ backdrop }: { backdrop: boolean }) {
  return (
    <div
      // 読み込み中の輪は押せる要素を持たないので、下のプレイヤー（再生バー）へ通す。
      className={cn(
        "pointer-events-none flex w-full items-center justify-center",
        backdrop && "bg-navbar",
      )}
    >
      <LoaderCircle className={cn("size-7 text-accent", spin)} aria-hidden="true" />
      <span role="status" className="sr-only">
        読み込み中
      </span>
    </div>
  );
}

/** PlaybackFailure は再生失敗と、失敗した位置からの再試行である。 */
export function PlaybackFailure({
  positionMs,
  onRetry,
}: {
  positionMs: number;
  onRetry: () => void;
}) {
  return (
    <Dimmed>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <AlertCircle className="size-8 text-danger" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-fg">再生できませんでした</h2>
        <p className="text-sm text-fg-muted text-balance">
          ファイルが移動・削除されたか、ブラウザが対応していない形式の可能性があります。
        </p>
        <Button variant="secondary" onClick={onRetry} className="mt-1">
          <RotateCcw aria-hidden="true" />
          {formatDuration(positionMs)} からもう一度試す
        </Button>
      </Panel>
    </Dimmed>
  );
}

const stageIcons: Record<StageState, ReactNode> = {
  done: <CircleCheck className="size-4 shrink-0 text-success" aria-hidden="true" />,
  active: (
    <LoaderCircle
      className={cn("size-4 shrink-0 text-accent", spin)}
      aria-hidden="true"
    />
  ),
  waiting: <Circle className="size-4 shrink-0 text-fg-muted" aria-hidden="true" />,
  failed: <AlertCircle className="size-4 shrink-0 text-warning" aria-hidden="true" />,
};

const stageLabels: Record<StageState, string> = {
  done: "完了",
  active: "処理中",
  waiting: "待機中",
  failed: "作成できませんでした",
};

/** ProcessingStages は取り込み中（読み取り前）の段階表示である（要件 12）。 */
export function ProcessingStages({ video }: { video: Video }) {
  return (
    <Surface>
      <div className="flex w-full max-w-sm flex-col gap-3 text-left">
        <h2 className="text-lg font-semibold text-fg">再生の準備をしています</h2>
        <p className="hidden text-sm text-fg-muted sm:block">
          動画の情報を読み取っています。終わるとこの画面のまま再生できるようになります。
        </p>
        <div role="status" aria-live="polite">
          <ol className="flex flex-col gap-1.5 sm:gap-3">
            {processingStages(video).map((stage) => (
              <li key={stage.name} className="flex items-center gap-2.5">
                {stageIcons[stage.state]}
                <span
                  className={cn(
                    "min-w-0 flex-1 text-sm",
                    stage.state === "active" && "font-medium text-fg",
                    stage.state === "done" && "text-fg",
                    (stage.state === "waiting" || stage.state === "failed") &&
                      "text-fg-muted",
                  )}
                >
                  {stage.name}
                </span>
                <span
                  className={cn(
                    "shrink-0 text-xs",
                    stage.state === "active" ? "text-accent" : "text-fg-muted",
                  )}
                >
                  {stageLabels[stage.state]}
                </span>
              </li>
            ))}
          </ol>
        </div>
        <p className="hidden text-xs text-fg-muted sm:block">
          再生できるのは『動画情報の読み取り』が終わってからです。残りは再生中に作られます。
        </p>
      </div>
    </Surface>
  );
}

/**
 * ReadFailure は読み取りに失敗した動画の表示である（要件 13）。
 *
 * `onReprobe` は、やり直しを受け付けたら（202 か 409 `probe_not_failed`）動画を取り直して
 * 解決し、それ以外の失敗では reject する。取り直しで段階表示へ移るまでボタンは押せない。
 */
export function ReadFailure({
  video,
  onReprobe,
}: {
  video: Video;
  onReprobe: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const { open, failure: openFailure } = useOpenFile(video.id);
  const openable = video.location?.openable === true;

  const reprobe = () => {
    setBusy(true);
    setFailed(false);
    void onReprobe()
      .catch(() => setFailed(true))
      .finally(() => setBusy(false));
  };

  return (
    <Surface>
      <Panel role="alert" className="max-w-lg gap-3">
        <AlertTriangle className="size-8 text-warning" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-fg">この動画を読み取れませんでした</h2>
        <p className="text-sm text-fg-muted">
          ファイルが壊れているか、途中までしか書き込まれていない可能性があります。
        </p>
        {video.probeError !== undefined && video.probeError !== "" && (
          <pre className="max-h-[calc(3lh+1rem)] overflow-y-auto rounded-md border border-border bg-field px-3 py-2 font-mono text-xs whitespace-pre-wrap break-all text-fg">
            {video.probeError}
          </pre>
        )}
        <div className="mt-1 flex flex-wrap gap-2">
          <Button variant="secondary" onClick={reprobe} disabled={busy}>
            {busy ? (
              <LoaderCircle className={spin} aria-hidden="true" />
            ) : (
              <RefreshCw aria-hidden="true" />
            )}
            もう一度読み取る
          </Button>
          {openable && (
            <Button variant="secondary" onClick={open}>
              <FolderOpen aria-hidden="true" />
              ファイルを開く
            </Button>
          )}
        </div>
        {failed && <p className="text-sm text-danger">読み取りを始められませんでした</p>}
        {openFailure !== null && <p className="text-sm text-danger">{openFailure}</p>}
      </Panel>
    </Surface>
  );
}

/** MissingVideo は表示中の動画が無いとき（404）の表示である。戻る操作は × に任せる。 */
export function MissingVideo() {
  return (
    <Surface>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <AlertCircle className="size-8 text-fg-muted" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-fg">この動画は開けません</h2>
        <p className="text-sm text-fg-muted text-balance">
          ライブラリから外れたか、ファイルが無くなりました。
        </p>
      </Panel>
    </Surface>
  );
}

/**
 * LoadFailure は、最初の取得が 404 以外で失敗したときの表示である。一時的な失敗かも
 * しれないので、取り直す「再試行」を置く。
 */
export function LoadFailure({
  reason,
  onRetry,
}: {
  reason: string;
  onRetry: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const retry = () => {
    setBusy(true);
    void onRetry().finally(() => setBusy(false));
  };
  return (
    <Surface>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <AlertCircle className="size-8 text-danger" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-fg">この動画を読み込めませんでした</h2>
        <p className="text-sm text-fg-muted text-balance">{reason}</p>
        <Button variant="secondary" onClick={retry} disabled={busy} className="mt-1">
          {busy ? (
            <LoaderCircle className={spin} aria-hidden="true" />
          ) : (
            <RefreshCw aria-hidden="true" />
          )}
          再試行
        </Button>
      </Panel>
    </Surface>
  );
}

/** Unplayable は、読み取れたが再生に必要な情報（長さ・映像）が無い動画の表示である。 */
export function Unplayable() {
  return (
    <Surface>
      <Panel role="alert" className="max-w-md items-center gap-3 text-center">
        <AlertTriangle className="size-8 text-warning" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-fg">この動画は再生できません</h2>
        <p className="text-sm text-fg-muted text-balance">
          再生に必要な長さや映像の情報がありません。
        </p>
      </Panel>
    </Surface>
  );
}

/** CreatingLine はプレイヤー直下の作成中の 1 行である（要件 11）。 */
export function CreatingLine({ video }: { video: Video }) {
  const line = creatingLine(video);
  if (line === null) return null;
  return (
    <p role="status" className="flex items-center gap-2 text-xs text-fg-muted">
      <LoaderCircle
        className={cn("size-3.5 shrink-0 text-accent", spin)}
        aria-hidden="true"
      />
      {line}
    </p>
  );
}
