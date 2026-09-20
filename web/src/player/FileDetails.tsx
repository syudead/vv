import * as Tabs from "@radix-ui/react-tabs";

import type { Video } from "../api/client";
import {
  formatBytes,
  formatDateTime,
  formatDuration,
  formatResolution,
  qualityLabel,
} from "../lib/format";

const probeLabel: Record<Video["probeState"], string> = {
  pending: "解析待ち",
  done: "解析済み",
  failed: "解析失敗",
};

function Rows({ rows }: { rows: { label: string; value: string }[] }) {
  return (
    <dl className="grid grid-cols-[max-content_1fr] gap-x-8 gap-y-2 text-sm">
      {rows
        .filter((row) => row.value !== "")
        .map((row) => (
          <div key={row.label} className="contents">
            <dt className="text-fg-muted">{row.label}</dt>
            <dd className="min-w-0 break-all text-fg tabular-nums">{row.value}</dd>
          </div>
        ))}
    </dl>
  );
}

const tabClass =
  "border-b-2 border-transparent px-2 py-2 text-sm text-fg transition-colors hover:border-fg " +
  "data-[state=active]:border-link data-[state=active]:text-link";

/** FileDetails は Stash の scene-tabs と同じ、プレイヤー下のタブ。 */
export default function FileDetails({ video }: { video: Video }) {
  const details = [
    { label: "長さ", value: formatDuration(video.durationMs) },
    { label: "画質", value: qualityLabel(video) },
    { label: "追加日時", value: formatDateTime(video.addedAt) },
    {
      label: "最終再生",
      value: video.progress === undefined ? "" : formatDateTime(video.progress.updatedAt),
    },
    {
      label: "再生位置",
      value:
        video.progress === undefined
          ? ""
          : video.progress.completed
            ? "最後まで再生"
            : formatDuration(video.progress.positionMs),
    },
  ];

  const file = [
    { label: "解像度", value: formatResolution(video) },
    { label: "コンテナ", value: video.container ?? "" },
    { label: "映像コーデック", value: video.videoCodec ?? "" },
    { label: "音声コーデック", value: video.audioCodec ?? "" },
    {
      label: "大きさ",
      value: `${formatBytes(video.sizeBytes)}（${video.sizeBytes.toLocaleString("ja-JP")} バイト）`,
    },
    {
      label: "解析",
      value:
        video.probeState === "failed" && video.probeError !== undefined
          ? `${probeLabel.failed}: ${video.probeError}`
          : probeLabel[video.probeState],
    },
    { label: "ブラウザ再生", value: video.playable ? "可" : "不可" },
    { label: "ID", value: String(video.id) },
  ];

  return (
    <Tabs.Root defaultValue="details" className="flex flex-col gap-4">
      <Tabs.List
        className="flex gap-1 border-b border-border-strong"
        aria-label="動画の情報"
      >
        <Tabs.Trigger value="details" className={tabClass}>
          詳細
        </Tabs.Trigger>
        <Tabs.Trigger value="file" className={tabClass}>
          ファイル情報
        </Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="details" className="outline-none">
        <Rows rows={details} />
      </Tabs.Content>
      <Tabs.Content value="file" className="outline-none">
        <Rows rows={file} />
      </Tabs.Content>
    </Tabs.Root>
  );
}
