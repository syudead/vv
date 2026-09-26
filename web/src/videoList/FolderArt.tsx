import {
  type PointerEvent,
  type ReactNode,
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
 * 読み込みを始めるときに onStart を呼ぶ（一覧の同時に1件の調整に知らせる）。
 */
function SheetPreview({ src, onStart }: { src: string; onStart?: () => void }) {
  const [armed, setArmed] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [failed, setFailed] = useState(false);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  // 待ち時間は前に出たときに1回だけ数える。onStart が変わっても数え直さない。
  const onStartRef = useRef(onStart);
  useLayoutEffect(() => {
    onStartRef.current = onStart;
  });

  useEffect(() => {
    const timer = setTimeout(() => {
      onStartRef.current?.();
      setArmed(true);
    }, previewDelayMs);
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
 * 置き場所（`relative` で 16:9 の箱）いっぱいに描く。先頭の4件だけを重ねる。
 *
 * ライブラリの格子では、動画のカードと同じ一覧のプレビューの調整
 * （usePreviewCoordination）に加わる。`onPreviewStart` を渡すと、流し始めた1枚の
 * 動画の id を知らせ、ほかのカードが流し始めたとき（`activePreviewId` が変わる）と
 * 止める指示（`previewResetEpoch` が進む）で前の1枚を戻す。選択中（`selectionMode`）は
 * 前に出さない。
 */
export default function FolderArt({
  previews,
  selectionMode = false,
  activePreviewId,
  previewResetEpoch,
  onPreviewStart,
}: {
  previews: readonly FolderPreview[];
  selectionMode?: boolean;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
}) {
  const shown = previews.slice(0, 4);
  const layout = sheetLayout[shown.length] ?? [];
  // 取り込み後の再取得で件数が減って範囲外になった位置は、どの1枚にも当たらない。
  const [front, setFront] = useState<number | null>(null);
  // 一覧の調整に知らせて流している1枚の動画の id。
  const [playingId, setPlayingId] = useState<number | null>(null);
  const observedResetEpoch = useRef(previewResetEpoch);
  const observedActiveId = useRef(activePreviewId);

  const putBack = useCallback(() => {
    setFront(null);
    setPlayingId(null);
  }, []);

  useEffect(() => {
    const resetChanged = observedResetEpoch.current !== previewResetEpoch;
    observedResetEpoch.current = previewResetEpoch;
    // ほかのカードが流し始めたこと（流している1枚と違う id への変化）だけで戻す。
    // 知らせた id が届く前の古い値では戻さない。
    const activeChanged = observedActiveId.current !== activePreviewId;
    observedActiveId.current = activePreviewId;
    const replaced =
      onPreviewStart !== undefined &&
      activeChanged &&
      playingId !== null &&
      activePreviewId !== playingId;
    if (resetChanged || selectionMode || replaced) putBack();
  }, [
    activePreviewId,
    onPreviewStart,
    playingId,
    previewResetEpoch,
    putBack,
    selectionMode,
  ]);

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (selectionMode || shown.length === 0 || event.pointerType !== "mouse") {
      return;
    }
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    const ratio = (event.clientX - rect.left) / rect.width;
    const next = Math.min(
      shown.length - 1,
      Math.max(0, Math.floor(ratio * shown.length)),
    );
    if (next === front) return;
    setFront(next);
    setPlayingId(null);
  };

  return (
    <div
      aria-hidden="true"
      data-folder-art=""
      onPointerMove={onPointerMove}
      onPointerLeave={putBack}
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
                <SheetPreview
                  key={preview.videoId}
                  src={preview.previewUrl}
                  onStart={() => {
                    setPlayingId(preview.videoId);
                    onPreviewStart?.(preview.videoId);
                  }}
                />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

/**
 * FolderStrip はリスト表示の行のフォルダの絵柄である。タブ付きの背板の中に、`previews` を
 * 傾けず重ねず同じ大きさで横一列に並べる。幅は並べた分だけで、置き場所の幅
 * （`max-w-*` など）を超える分は折り返されて1行の高さの外に出るので見えない
 * （入る分だけ並べ、入らない分は省く）。下見はしない。装飾なので読み上げない。
 * `children` は背板の下端に重ねるもの（見ている途中の帯など）である。
 */
export function FolderStrip({
  previews,
  children,
}: {
  previews: readonly FolderPreview[];
  children?: ReactNode;
}) {
  return (
    <div aria-hidden="true" data-folder-art="" className="relative max-w-full pt-1.5">
      <div className="absolute top-0 left-0 h-2 w-12 rounded-t-sm bg-elevated" />
      <div className="relative overflow-hidden rounded-sm rounded-tl-none bg-elevated p-1">
        <div className="flex h-9 min-w-16 flex-wrap gap-x-1 gap-y-2 overflow-hidden">
          {previews.map((preview) => (
            <div
              key={preview.videoId}
              data-folder-preview=""
              className="aspect-video h-full shrink-0 overflow-hidden rounded-[2px] border border-border-strong bg-navbar"
            >
              <img
                src={preview.thumbnailUrl}
                alt=""
                loading="lazy"
                decoding="async"
                className="h-full w-full object-contain"
              />
            </div>
          ))}
        </div>
        {children}
      </div>
    </div>
  );
}
