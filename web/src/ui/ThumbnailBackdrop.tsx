/**
 * ThumbnailBackdrop は、縦長の動画のサムネイルの後ろに敷く、同じ画像をぼかした背景である。
 *
 * 16:9 の枠に縦長の画像を切り抜かずに収めると左右に大きな余白が出て、縦長の動画ばかりが
 * 並ぶと一覧が空いて見える。余白をその画像の色で埋めて、横長の動画と同じ密度に見せる。
 * 親は `relative` と `overflow-hidden` を持ち、手前の画像は `relative` で重ねる。
 */
export default function ThumbnailBackdrop({ src }: { src: string }) {
  return (
    <img
      src={src}
      alt=""
      aria-hidden="true"
      loading="lazy"
      decoding="async"
      data-thumbnail-backdrop=""
      className="pointer-events-none absolute inset-0 h-full w-full scale-125 object-cover opacity-50 blur-xl"
    />
  );
}
