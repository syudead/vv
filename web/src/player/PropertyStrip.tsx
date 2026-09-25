import type { Video } from "../api/client";
import { cn } from "../lib/cn";
import { videoProperties } from "./properties";

/**
 * PropertyStrip は題名の下の区切り線と、その下の属性の一列である（要件 17）。
 *
 * `xl` 以上は縦線で区切った折り返さない 1 行、それ未満は線の無い格子にする。折り返す幅で
 * 縦線を使うと、行頭に線と余白が残るからである（ui-design）。
 */
export default function PropertyStrip({
  video,
  lastPlayed = true,
}: {
  video: Video;
  /** LAST PLAYED を出すか。再生位置は所有者のものなので、ゲストでは省く。 */
  lastPlayed?: boolean;
}) {
  return (
    <div className="border-t border-border pt-5">
      <dl className="grid grid-cols-3 gap-x-6 gap-y-4 sm:grid-cols-4 xl:flex xl:flex-nowrap xl:gap-0">
        {videoProperties(video, { lastPlayed }).map((property) => (
          <div
            key={property.label}
            className="flex min-w-0 flex-col gap-1 xl:border-l xl:border-border xl:px-4 xl:first:border-l-0 xl:first:pl-0"
          >
            <dt
              lang="en"
              className="text-xs font-medium tracking-wider text-fg-muted uppercase"
            >
              {property.label}
            </dt>
            <dd
              title={property.value}
              className={cn(
                // xl の 1 行では、長い値を省略して横スクロールを出さない（全体は title）。
                "text-sm font-medium tabular-nums [overflow-wrap:anywhere] xl:truncate",
                property.tone === "muted"
                  ? "text-fg-muted"
                  : property.tone === "warning"
                    ? "text-warning"
                    : "text-fg",
              )}
            >
              {property.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
