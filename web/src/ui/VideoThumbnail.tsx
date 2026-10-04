import type * as React from "react";

import { cn } from "@/lib/cn";
import { Progress } from "@/ui/shadcn/progress";

// VideoThumbnail は vv 固有の部品で、動画・まとめ・フォルダのカードと一覧の行が共有する
// サムネイルの枠である（specs/038-design-system/ui-design.md「Components」、
// web/registry/rules/components.md「VideoThumbnail」）。shadcn/ui の部品と同じく、
// 枠と、その上に重ねる部品（長さの印・視聴の進み・角の印・全面の知らせ）に分けて組む。
//
// 重ねる順（下から）: 画像（children）→ 長さの印 → 進みの縁 → 全面の知らせ → 角の印。

/** VideoThumbnail は 16:9 の枠で、中身（画像・下見の動画）を children に受ける。 */
function VideoThumbnail({
  className,
  selected = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  /** 選択中なら枠を primary の輪で囲む。 */
  selected?: boolean;
}) {
  return (
    <div
      data-slot="video-thumbnail"
      data-selected={selected || undefined}
      className={cn(
        "group/video-thumbnail relative aspect-video w-full overflow-hidden rounded-md bg-navbar",
        "data-selected:ring-2 data-selected:ring-primary",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

/** VideoThumbnailImage は枠いっぱいに収める画像である。縦長の絵は切り抜かず左右を空ける。 */
function VideoThumbnailImage({
  className,
  alt = "",
  ...props
}: React.ComponentProps<"img">) {
  return (
    <img
      data-slot="video-thumbnail-image"
      alt={alt}
      loading="lazy"
      decoding="async"
      className={cn("relative size-full object-contain", className)}
      {...props}
    />
  );
}

/**
 * VideoThumbnailDuration は右下の長さの印である。公開の印などの小さなアイコンを前に並べられる。
 * 文字は thumbnails 用の `text-2xs`、面は `bg-overlay`。
 */
function VideoThumbnailDuration({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="video-thumbnail-duration"
      className={cn(
        "absolute right-2 bottom-2 flex items-center gap-1.5 rounded-sm bg-overlay px-1.5 py-0.5 text-2xs font-medium text-foreground tabular-nums [&_svg]:size-3",
        className,
      )}
      {...props}
    />
  );
}

/**
 * VideoThumbnailProgress は下端の視聴の進みの縁である。`value` は 0〜100。読み上げ名は
 * 呼び手が `aria-label` で渡す。
 */
function VideoThumbnailProgress({
  className,
  ...props
}: React.ComponentProps<typeof Progress>) {
  return (
    <Progress
      data-slot="video-thumbnail-progress"
      className={cn("absolute inset-x-0 bottom-0 rounded-none bg-overlay", className)}
      {...props}
    />
  );
}

/**
 * VideoThumbnailMark は角に置く印（選択のチェック、お気に入りのハート）の置き場である。
 * 画像の上でも読めるよう、中のアイコンには `drop-shadow-mark` を付ける。
 */
function VideoThumbnailMark({
  className,
  corner = "top-end",
  ...props
}: React.ComponentProps<"div"> & { corner?: "top-start" | "top-end" }) {
  return (
    <div
      data-slot="video-thumbnail-mark"
      data-corner={corner}
      className={cn(
        "absolute top-1.5 z-20 flex data-[corner=top-end]:right-1.5 data-[corner=top-start]:left-1.5 [&_svg]:drop-shadow-mark",
        className,
      )}
      {...props}
    />
  );
}

/** VideoThumbnailNotice は枠全体を覆う知らせ（再生できない理由など）である。 */
function VideoThumbnailNotice({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="video-thumbnail-notice"
      className={cn(
        "absolute inset-0 flex flex-col items-center justify-center gap-1.5 bg-overlay px-3 text-center text-xs font-medium text-warning [&_svg]:size-5",
        className,
      )}
      {...props}
    />
  );
}

export {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailMark,
  VideoThumbnailNotice,
  VideoThumbnailProgress,
};
