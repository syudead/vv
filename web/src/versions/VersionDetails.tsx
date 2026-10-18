import type { Video } from "../api/client";
import { t } from "../i18n";
import { formatBytes, formatResolution } from "../lib/format";
import { formatCodec } from "../player/properties";

export interface VersionDetailValues {
  specs: string[];
  /** Empty when the root display name or relative location is unknown. */
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

export function versionDetailsText(
  details: VersionDetailValues,
  separator: string,
  place = details.place,
): string {
  return [...details.specs, place].filter((value) => value !== "").join(separator);
}

export function VersionDetailsLine({
  details,
  title,
}: {
  details: VersionDetailValues;
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
          {/* Preserve the path suffix nearest the file. */}
          <span dir="rtl" className="min-w-0 truncate text-left">
            <bdi dir="ltr">{details.place}</bdi>
          </span>
        </>
      )}
    </span>
  );
}
