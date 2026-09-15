import type { Video } from "../api/client";
import { formatDuration } from "./VideoCard";

/**
 * missing は取れていない値の見せ方を決める（data-model.md 4.「取れていないとき」
 * / FR-016）。
 *
 * **空欄にしない。** 空欄は「値が無い」のか「まだ調べていない」のか「調べたが
 * 読めなかった」のかを区別できず、利用者は待てばよいのか諦めるのかを判断
 * できない。
 *
 * 文言は 004 のまま変えない。005 が変えるのは体裁だけである（R-507）。
 */
function missing(video: Video): string {
  if (video.probeState === "pending") {
    return "確認中";
  }
  if (video.probeState === "failed") {
    return video.probeError === undefined
      ? "読み取れませんでした"
      : `読み取れませんでした (${video.probeError})`;
  }
  // 解析は済んでいて値が無い。音声の無い動画のように、無いこと自体が情報である。
  return "なし";
}

/** formatSize はバイト数を読める大きさにする。 */
function formatSize(bytes: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit] ?? "B"}`;
}

/**
 * MetaList は情報パネルの中の動画の情報である（C16 / FR-016 / R-507）。
 *
 * 004 の `VideoFacts` をそのまま移したもので、**値・順序・言い分けは変えない**
 * （data-model.md 4.）。変わるのは体裁だけである。
 *
 * - 値から `font-mono` を外す。FR-016 が「等幅フォントの羅列」を名指しで
 *   禁じている。外して困るのは解像度（`1920 × 1080`）の桁揃えだが、項目が縦に
 *   1 つずつ並ぶ以上、揃える相手がいない。
 * - ラベルは `muted`、値は `body`。同じ濃さで並べると、どちらが項目名なのかを
 *   位置だけで読み取ることになる。
 * - **1 項目 1 行**にし、行間に区切り線を置く。パネルは幅 `18rem`〜`24rem` の
 *   縦長なので、項目名の列と値の列に割るより、行で区切るほうが値に幅が残る
 *   （値は「読み取れませんでした (理由)」のように長くなりうる）。
 *
 * `<dl>` / `<dt>` / `<dd>` は保つ。読み上げにラベルと値の対が伝わる構造が
 * すでにあり、見た目のために `<div>` の並びへ崩すと 004 が得ていたものを失う。
 */
export default function MetaList({ video }: { video: Video }) {
  const fallback = missing(video);

  /** value は取れていれば値を、取れていなければ言い分けを返す。 */
  const value = (text: string | undefined): string =>
    text === undefined || text === "" ? fallback : text;

  const resolution =
    video.width === undefined || video.height === undefined
      ? undefined
      : `${String(video.width)} × ${String(video.height)}`;

  // sizeBytes は必須の項目なので、この言い分けに入らない。
  const facts: [string, string][] = [
    ["長さ", value(formatDuration(video.durationMs))],
    ["解像度", value(resolution)],
    ["形式", value(video.container)],
    ["映像", value(video.videoCodec)],
    ["音声", value(video.audioCodec)],
    ["大きさ", formatSize(video.sizeBytes)],
  ];

  return (
    <dl className="divide-y divide-border/60 text-sm">
      {facts.map(([label, text]) => (
        // 1 行の中では項目名を左、値を右に寄せる。値が長ければ折り返して
        // 2 行目以降も値の側に収まる（min-w-0 が無いと折り返さずにはみ出す）。
        <div key={label} className="flex items-baseline justify-between gap-4 py-2">
          <dt className="shrink-0 text-muted">{label}</dt>
          <dd className="min-w-0 text-right break-words text-body">{text}</dd>
        </div>
      ))}
    </dl>
  );
}
