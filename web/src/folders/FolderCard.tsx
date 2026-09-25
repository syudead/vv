import {
  memo,
  type PointerEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { Link } from "react-router";

import type { FolderSummary } from "../api/client";
import { cn } from "../lib/cn";
import Skeleton from "../ui/Skeleton";
import { folderUrl } from "./folderPath";

type Preview = FolderSummary["previews"][number];

/**
 * sheetLayout は差し込むサムネイルの位置（背板に対する %）と傾き（度）である。
 * 件数ごとに左右へ均等に広げ、奇数番目をわずかに高く置く。前板が無いので、
 * 束が背板の上下の中央に来る高さに置く（ui-design.md）。
 */
const sheetLayout: Record<number, { left: number; top: number; rotate: number }[]> = {
  1: [{ left: 16, top: 14, rotate: 0 }],
  2: [
    { left: 5, top: 16, rotate: -3 },
    { left: 27, top: 12, rotate: 3 },
  ],
  3: [
    { left: 3, top: 16, rotate: -4 },
    { left: 16, top: 12, rotate: 0 },
    { left: 29, top: 16, rotate: 4 },
  ],
  4: [
    { left: 2, top: 16, rotate: -5 },
    { left: 11, top: 12, rotate: -1.5 },
    { left: 20, top: 16, rotate: 1.5 },
    { left: 30, top: 12, rotate: 5 },
  ],
};

/** frontPlace は指した1枚を前に出すときの位置（背板の中央）である。 */
const frontPlace = { left: 16, top: 6 };

/** previewDelayMs は前に出た1枚が動画を流し始めるまでの待ち時間（動画カードと同じ）。 */
const previewDelayMs = 400;

/**
 * SheetPreview は前に出た1枚の上で、一覧用のプレビュー動画を無音でくり返し流す。
 * 前に出てから previewDelayMs 待って読み込み、流れ始めるまではサムネイルを見せる。
 * 読めなければ何も出さず、サムネイルのまま残す。外れるときは読み込みを止める。
 */
function SheetPreview({ src }: { src: string }) {
  const [armed, setArmed] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [failed, setFailed] = useState(false);
  const videoRef = useRef<HTMLVideoElement | null>(null);

  useEffect(() => {
    const timer = setTimeout(() => setArmed(true), previewDelayMs);
    return () => clearTimeout(timer);
  }, []);

  const release = useCallback(() => {
    const element = videoRef.current;
    if (element === null) return;
    element.pause();
    element.removeAttribute("src");
    element.load();
  }, []);

  // レイアウトの後始末は ref が外れる前に走るので、ここで読み込みを止められる。
  useLayoutEffect(() => release, [release]);

  useEffect(() => {
    if (!armed || failed) return;
    const element = videoRef.current;
    if (element === null) return;
    // 再生を断られたら、要素を外す前に読み込みを解く（外した後は ref が届かない）。
    const fail = () => {
      release();
      setFailed(true);
    };
    try {
      element.play()?.catch(fail);
    } catch {
      fail();
    }
  }, [armed, failed, release]);

  if (!armed || failed) return null;
  return (
    <video
      ref={videoRef}
      src={src}
      data-folder-video=""
      muted
      playsInline
      loop
      preload="auto"
      controls={false}
      tabIndex={-1}
      onPlaying={() => setPlaying(true)}
      onError={() => {
        release();
        setFailed(true);
      }}
      className={cn(
        "absolute inset-0 h-full w-full bg-navbar object-contain",
        playing ? "opacity-100" : "opacity-0",
      )}
    />
  );
}

/**
 * FolderArt はタブ付きのフォルダの外形と、背板に斜めに重ねたサムネイルを描く。
 * マウスを横に動かすと、位置に応じた1枚が傾きを戻し、少し大きくなって中央の
 * 最前面に出て、プレビュー動画があればそれを流す（フォルダの中身の下見）。
 * 装飾なので読み上げない。
 */
function FolderArt({ previews }: { previews: Preview[] }) {
  const shown = previews.slice(0, 4);
  const layout = sheetLayout[shown.length] ?? [];
  // 取り込み後の再取得で件数が減って範囲外になった位置は、どの1枚にも当たらない。
  const [front, setFront] = useState<number | null>(null);

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (shown.length === 0 || event.pointerType !== "mouse") return;
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    const ratio = (event.clientX - rect.left) / rect.width;
    setFront(Math.min(shown.length - 1, Math.max(0, Math.floor(ratio * shown.length))));
  };

  return (
    <div
      aria-hidden="true"
      data-folder-art=""
      onPointerMove={onPointerMove}
      onPointerLeave={() => setFront(null)}
      className="absolute inset-x-3 top-3 bottom-2"
    >
      <div className="absolute top-0 left-0 h-3 w-2/5 rounded-t-md bg-elevated" />
      <div className="absolute inset-x-0 top-2 bottom-0 overflow-hidden rounded-md rounded-tl-none bg-elevated">
        {shown.map((preview, index) => {
          const place = layout[index];
          if (place === undefined) return null;
          const isFront = front === index;
          return (
            <div
              key={preview.videoId}
              data-folder-preview=""
              data-folder-front={isFront ? "" : undefined}
              className={cn(
                "absolute aspect-video w-[68%] overflow-hidden rounded-sm border border-border-strong bg-navbar transition-[left,top,transform,box-shadow] duration-200 ease-out-quart motion-reduce:transition-none",
                isFront ? "z-10 shadow-card-hover" : "shadow-card",
              )}
              style={{
                left: `${String(isFront ? frontPlace.left : place.left)}%`,
                top: `${String(isFront ? frontPlace.top : place.top)}%`,
                transform: isFront ? "scale(1.12)" : `rotate(${String(place.rotate)}deg)`,
              }}
            >
              <img
                src={preview.thumbnailUrl}
                alt=""
                loading="lazy"
                decoding="async"
                className="h-full w-full object-contain"
              />
              {isFront && preview.previewUrl !== undefined && (
                <SheetPreview key={preview.videoId} src={preview.previewUrl} />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** folderLabel はフォルダカードの読み上げ名である（親 Issue のアクセシビリティ）。 */
export function folderLabel(folder: FolderSummary, withPath: boolean): string {
  const label = `${folder.name}、動画 ${String(folder.videoCount)} 本、フォルダ ${String(folder.folderCount)} 件`;
  return withPath ? `${label}、${folder.rootPath}` : label;
}

/**
 * FolderCard はフォルダ1件のカードである。箱は動画カードと同じで、上半分だけが
 * フォルダの絵柄になる。showPath は最上位（登録フォルダ）でパスを添えるとき。
 */
function FolderCard({ folder, showPath }: { folder: FolderSummary; showPath: boolean }) {
  return (
    <article
      data-folder-path={folder.path}
      className="group relative flex flex-col overflow-hidden rounded-lg bg-surface shadow-card transition-[box-shadow,transform] duration-200 ease-out-quart hover:-translate-y-0.5 hover:shadow-card-hover has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-offset-2 has-[a:focus-visible]:outline-link motion-reduce:transition-none motion-reduce:hover:translate-y-0"
    >
      <Link
        to={folderUrl({ rootId: folder.rootId, path: folder.path })}
        aria-label={folderLabel(folder, showPath)}
        className="flex min-w-0 flex-col outline-none"
      >
        <div className="relative aspect-video w-full">
          <FolderArt previews={folder.previews} />
        </div>
        <div className="flex min-w-0 flex-col gap-1 px-3 pt-2 pb-3">
          <h3
            title={folder.name}
            className="line-clamp-2 text-sm leading-5 font-medium break-all text-fg"
          >
            {folder.name}
          </h3>
          <p className="text-xs text-fg-muted tabular-nums">
            動画 {folder.videoCount.toLocaleString("ja-JP")} 本
            <span className="text-fg-subtle"> · </span>
            フォルダ {folder.folderCount.toLocaleString("ja-JP")} 件
          </p>
          {showPath && (
            // 同名の登録フォルダを見分けるのはパスの末尾なので、先頭の側を省略する。
            <p
              title={folder.rootPath}
              dir="rtl"
              className="truncate text-left text-xs text-fg-muted"
            >
              <bdi dir="ltr">{folder.rootPath}</bdi>
            </p>
          )}
        </div>
      </Link>
    </article>
  );
}

export default memo(FolderCard);

/** FolderCardSkeleton は読み込み中のフォルダカードである。 */
export function FolderCardSkeleton({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, index) => (
        <div
          key={index}
          aria-hidden="true"
          className="flex flex-col overflow-hidden rounded-lg bg-surface"
        >
          <div className="relative aspect-video w-full">
            <div className="absolute inset-x-3 top-3 bottom-2">
              <Skeleton className="absolute top-0 left-0 h-3 w-2/5 rounded-b-none" />
              <Skeleton className="absolute inset-x-0 top-2 bottom-0 rounded-tl-none" />
            </div>
          </div>
          <div className="flex flex-col gap-1.5 px-3 pt-2 pb-3">
            <Skeleton className="h-4 w-3/5" />
            <Skeleton className="h-3 w-2/5" />
          </div>
        </div>
      ))}
    </>
  );
}
