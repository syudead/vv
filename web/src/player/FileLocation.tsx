import { AlertCircle, Folder } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { openVideoFile, RequestFailed, type VideoLocation } from "../api/client";
import { splitPath } from "./properties";

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
 * FileLocation は属性の下のファイルの場所の 1 行である（要件 18）。
 *
 * 開ける環境（`openable`）のときだけ、パス全体を 1 つのボタンにする。押すとサーバーの PC で
 * 開くだけで移動はしないので、リンクではなくボタンにする。開けない環境では下線も hover も
 * 付けない、ただの文字にする。
 *
 * 開けなかったときは、この行のすぐ下に 1 行だけ出す。帯やトーストは使わない。
 */
export default function FileLocation({
  videoId,
  location,
}: {
  videoId: number;
  location: VideoLocation;
}) {
  const { open, failure } = useOpenFile(videoId);
  const { folder, name } = splitPath(location.path);

  const text = (
    <>
      <span className="text-fg-muted transition-colors group-hover:text-fg">
        {folder}
      </span>
      <span className="text-fg">{name}</span>
    </>
  );

  return (
    <div className="flex flex-col gap-2 text-sm">
      <div className="flex min-w-0 items-start gap-2">
        <Folder className="mt-0.5 size-4 shrink-0 text-fg-muted" aria-hidden="true" />
        {location.openable ? (
          <button
            type="button"
            onClick={open}
            title={location.path}
            aria-label={`ファイルを開く: ${location.path}`}
            className="group min-w-0 flex-1 text-left break-all underline decoration-border-strong underline-offset-4 transition-colors hover:decoration-fg-muted sm:truncate"
          >
            {text}
          </button>
        ) : (
          <p title={location.path} className="min-w-0 flex-1 break-all sm:truncate">
            {text}
          </p>
        )}
      </div>
      {failure !== null && (
        <p role="alert" className="flex items-center gap-2 text-danger">
          <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
          {failure}
        </p>
      )}
    </div>
  );
}
