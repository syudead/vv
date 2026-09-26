import {
  type PointerEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import type { FolderPreview } from "../api/client";
import { cn } from "../lib/cn";

// フォルダの絵柄は、フォルダ画面のフォルダカードとライブラリのグループのカード・行が
// 共有する（どちらの画面にも属さないので videoList に置く）。

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
 *
 * 置き場所（`relative` で 16:9 の箱）いっぱいに描く。`size="row"` はリスト表示の行の
 * 小さなサムネイルの枠に描く形で、外形を詰め、下見をしない。
 */
export default function FolderArt({
  previews,
  size = "card",
}: {
  previews: readonly FolderPreview[];
  size?: "card" | "row";
}) {
  const row = size === "row";
  const shown = previews.slice(0, 4);
  const layout = sheetLayout[shown.length] ?? [];
  // 取り込み後の再取得で件数が減って範囲外になった位置は、どの1枚にも当たらない。
  const [front, setFront] = useState<number | null>(null);

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (row || shown.length === 0 || event.pointerType !== "mouse") return;
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
      className={cn(
        "absolute",
        row ? "inset-x-1 inset-y-0.5" : "inset-x-3 top-3 bottom-2",
      )}
    >
      <div
        className={cn(
          "absolute top-0 left-0 w-2/5 bg-elevated",
          row ? "h-2 rounded-t-sm" : "h-3 rounded-t-md",
        )}
      />
      <div
        className={cn(
          "absolute inset-x-0 bottom-0 overflow-hidden bg-elevated",
          row ? "top-1.5 rounded-sm rounded-tl-none" : "top-2 rounded-md rounded-tl-none",
        )}
      >
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
