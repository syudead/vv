import * as Collapsible from "@radix-ui/react-collapsible";
import { ChevronDown } from "lucide-react";
import { useState } from "react";

import type { Video } from "../api/client";
import {
  formatBytes,
  formatDateTime,
  formatDuration,
  formatResolution,
} from "../lib/format";

const probeLabel: Record<Video["probeState"], string> = {
  pending: "解析待ち",
  done: "解析済み",
  failed: "解析失敗",
};

/** FileDetails は折りたたみ式の詳細メタ。既定は閉じている。 */
export default function FileDetails({ video }: { video: Video }) {
  const [open, setOpen] = useState(false);

  const rows: { label: string; value: string }[] = [
    { label: "解像度", value: formatResolution(video) },
    { label: "コンテナ", value: video.container ?? "" },
    { label: "映像コーデック", value: video.videoCodec ?? "" },
    { label: "音声コーデック", value: video.audioCodec ?? "" },
    { label: "長さ", value: formatDuration(video.durationMs) },
    {
      label: "大きさ",
      value: `${formatBytes(video.sizeBytes)}（${video.sizeBytes.toLocaleString("ja-JP")} バイト）`,
    },
    { label: "追加日時", value: formatDateTime(video.addedAt) },
    {
      label: "最終再生",
      value: video.progress === undefined ? "" : formatDateTime(video.progress.updatedAt),
    },
    {
      label: "解析",
      value:
        video.probeState === "failed" && video.probeError !== undefined
          ? `${probeLabel.failed}: ${video.probeError}`
          : probeLabel[video.probeState],
    },
    { label: "ID", value: String(video.id) },
  ].filter((row) => row.value !== "");

  return (
    <Collapsible.Root
      open={open}
      onOpenChange={setOpen}
      className="rounded-lg bg-surface"
    >
      <Collapsible.Trigger className="flex h-11 w-full items-center justify-between rounded-lg px-4 text-sm font-medium text-fg transition-colors hover:bg-surface-hover">
        ファイル情報
        <ChevronDown
          className={
            "size-4 text-fg-muted transition-transform duration-200 " +
            (open ? "rotate-180" : "")
          }
        />
      </Collapsible.Trigger>
      <Collapsible.Content className="px-4 pb-4">
        <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 pt-1 text-sm">
          {rows.map((row) => (
            <div key={row.label} className="contents">
              <dt className="text-fg-muted">{row.label}</dt>
              <dd className="min-w-0 break-all text-fg tabular-nums">{row.value}</dd>
            </div>
          ))}
        </dl>
      </Collapsible.Content>
    </Collapsible.Root>
  );
}
