import type { Video } from "../api/client";
import { t } from "../i18n";
import { formatBytes, formatResolution } from "../lib/format";
import { formatCodec } from "../player/properties";

/**
 * 同じ動画の別バージョンの行の 2 行目の値（specs/030-video-versions/ui-design.md
 * 「Versions list」）。再生画面のバージョンの一覧と、束ねる窓の代表の行が同じ書式で使う。
 */
export interface VersionDetailValues {
  /** 解像度・コンテナ・映像コーデック・サイズ。分からない値は省く。 */
  specs: string[];
  /** 「登録フォルダの表示名 / 相対パス」。分からなければ空。 */
  place: string;
}

export function versionDetails(video: Video): VersionDetailValues {
  const specs = [
    formatResolution(video),
    video.container?.toUpperCase(),
    video.videoCodec === undefined ? undefined : formatCodec(video.videoCodec),
    video.probeState === "done" ? formatBytes(video.sizeBytes) : undefined,
  ].filter((value): value is string => value !== undefined && value !== "");
  const parts: string[] = [];
  if (video.folder?.rootName !== undefined) parts.push(video.folder.rootName);
  if (video.folder !== undefined && video.folder.path !== "")
    parts.push(video.folder.path);
  return { specs, place: t.shell.scan.location(parts) };
}

/** versionDetailsText は 2 行目を 1 つの文字列にする（読み上げ名・`title` 用）。 */
export function versionDetailsText(
  details: VersionDetailValues,
  separator: string,
  place = details.place,
): string {
  return [...details.specs, place].filter((value) => value !== "").join(separator);
}

/**
 * VersionDetailsLine は 2 行目の表示である。項目の間は `text-muted-foreground` の「·」で、置き場所は
 * 末尾（ファイルに近い側）を優先して残し、行全体を 1 行で省略する。
 */
export function VersionDetailsLine({
  details,
  title,
}: {
  details: VersionDetailValues;
  /** 行の `title`。省略した分を読めるようにする。 */
  title: string;
}) {
  return (
    <span
      className="flex min-w-0 items-center gap-1.5 overflow-hidden text-xs whitespace-nowrap text-muted-foreground tabular-nums"
      title={title}
    >
      {details.specs.map((value, index) => (
        <span
          key={`${String(index)}:${value}`}
          className="flex shrink-0 items-center gap-1.5"
        >
          {index > 0 && (
            <span className="text-muted-foreground" aria-hidden="true">
              ·
            </span>
          )}
          {value}
        </span>
      ))}
      {details.place !== "" && (
        <>
          {details.specs.length > 0 && (
            <span className="shrink-0 text-muted-foreground" aria-hidden="true">
              ·
            </span>
          )}
          {/* 置き場所は末尾（ファイルに近い側）を優先して残す。 */}
          <span dir="rtl" className="min-w-0 truncate text-left">
            <bdi dir="ltr">{details.place}</bdi>
          </span>
        </>
      )}
    </span>
  );
}
