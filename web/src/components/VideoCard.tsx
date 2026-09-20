import { Link } from "react-router";

import type { Video } from "../api/client";
import Icon from "../layout/icons";

/** formatDuration は尺を mm:ss（1時間以上は h:mm:ss）で表す。 */
export function formatDuration(durationMs: number | undefined): string {
  if (durationMs === undefined || durationMs < 0) {
    return "";
  }

  const totalSeconds = Math.floor(durationMs / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  const pad = (value: number) => String(value).padStart(2, "0");
  if (hours > 0) {
    return `${String(hours)}:${pad(minutes)}:${pad(seconds)}`;
  }
  return `${String(minutes)}:${pad(seconds)}`;
}

/** unplayableText は再生できない理由を、利用者に伝わる言葉にする。 */
export function unplayableText(video: Video): string | null {
  if (video.playable) {
    return null;
  }
  if (video.probeState === "failed") {
    return "読み取れませんでした";
  }
  if (video.probeState === "pending") {
    return "確認中";
  }

  switch (video.unplayableReason) {
    case "container":
      return `${video.container ?? "この形式"} は再生できません`;
    case "video_codec":
      return `映像の形式 (${video.videoCodec ?? "不明"}) は再生できません`;
    case "audio_codec":
      return `音声の形式 (${video.audioCodec ?? "不明"}) は再生できません`;
    default:
      return "再生できません";
  }
}

/**
 * partialRatio は途中まで見た割合を返す。見ていない・見終わった・尺が
 * 分からない場合は null を返す（帯を描かない）。
 */
export function partialRatio(video: Video): number | null {
  const progress = video.progress;
  if (progress === undefined || progress.completed) {
    return null;
  }
  if (video.durationMs === undefined || video.durationMs <= 0) {
    return null;
  }
  const ratio = progress.positionMs / video.durationMs;
  if (ratio <= 0) {
    return null;
  }
  return Math.min(ratio, 1);
}

/**
 * states はカード全体の 4 状態である（005 の contracts/components.md 2. の C8）。
 *
 * hover と active は **`::after` の覆い**で表す。C3 NavItem と同じ手で、地の色に
 * 対して同じ向きに 1 段動く。`isolate` + `-z-10` で覆いを中身の下に敷くので、
 * 題名もサムネイル上の小片も覆われない。
 *
 * 動きを減らす設定では遷移だけを止め、**色の最終状態は常に適用する**（FR-012）。
 */
const states =
  "relative isolate after:pointer-events-none after:absolute after:inset-0 " +
  "after:-z-10 after:rounded-card after:transition-colors " +
  "hover:after:bg-body/10 active:after:bg-surface-sunken/60 outline-none " +
  "motion-reduce:after:transition-none " +
  "focus-visible:outline-none";

/**
 * VideoCard は一覧の1件を描く
 * （contracts/screen-states.md 1.「一覧の 1 項目」）。
 *
 * サムネイルと題名は**1 つの枠**にまとまり、角丸は枠全体に掛かる
 * （spec 4. / 005 の contracts/components.md 4.）。サムネイルは枠の上端に接する
 * ので、角丸はサムネイル側で上の 2 隅だけを切り、下の 2 隅は枠の地が描く。
 * **枠に `overflow-hidden` を掛けない** — 掛けると下の 3 つ目の到達手段
 * （狙いを合わせると省略が解ける題名）が枠の高さで切られてしまう。
 *
 * 枠は 16:9 固定である。サムネイルの有無で大きさが変わらない（SC-003）ので、
 * 画像が届いてもレイアウトが動かず、読んでいる位置が飛ばない。
 *
 * 題名の全文への到達手段は 2 つある（R-409 / FR-011）。
 *
 * 1. リンクのアクセシブル名は**常に全文**である（aria-label）。見た目の省略に
 *    引きずられない。
 * 2. ポインタには title 属性で全文を出す。
 * キーボード利用時も aria-label から全文を読み上げる。見た目は一行に固定し、
 * 長い題名だけで格子の高さが変わらないようにする（FR-005 / SC-003）。
 */
export default function VideoCard({
  video,
  backTo,
  selected = false,
  selectionMode = false,
  onSelect,
}: {
  video: Video;
  /**
   * backTo は遷移元の一覧の URL である（FR-016）。再生画面の「一覧へ戻る」が
   * ここへ帰る。渡さないと再生画面は `/` へ戻すので、検索語と並び順が消える。
   */
  backTo?: string;
  selected?: boolean;
  selectionMode?: boolean;
  onSelect?: (id: number, selected: boolean) => void;
}) {
  const duration = formatDuration(video.durationMs);
  const unplayable = unplayableText(video);
  const watched = video.progress?.completed === true;
  const watchedRatio = partialRatio(video);

  return (
    <article className="group relative">
      <input
        type="checkbox"
        data-video-select="true"
        checked={selected}
        onChange={(event) => onSelect?.(video.id, event.target.checked)}
        onClick={(event) => event.stopPropagation()}
        aria-label={`「${video.title}」を選択`}
        className={
          "peer absolute top-2 left-2 z-10 h-5 w-5 cursor-pointer appearance-none rounded-selection border border-body/80 bg-badge transition-[opacity,border-color,background-color] hover:border-accent checked:border-accent checked:bg-accent " +
          "outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus motion-reduce:transition-none " +
          (selectionMode || selected
            ? "opacity-100"
            : "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100")
        }
      />
      <span className="pointer-events-none absolute top-2 left-2 z-20 hidden h-5 w-5 items-center justify-center text-accent-ink peer-checked:flex">
        <Icon name="check" className="h-4 w-4" />
      </span>
      <Link
        to={`/videos/${String(video.id)}`}
        // 帰り道を持たせる。URL のクエリ（検索語・並び順）は一覧側にしか無いので、
        // ここで渡さないと再生画面は「どの一覧から来たか」を知りようがない。
        state={backTo === undefined ? undefined : { from: backTo }}
        // 見た目は省略しても、読み上げには必ず全文を渡す（R-409）。名前を
        // 中身から組み立てると、枠の中の小片（長さ・視聴済み）まで名前に
        // 混ざってしまう。
        aria-label={video.title}
        className={"group/card block cursor-pointer rounded-card " + states}
      >
        <div
          className={
            "relative aspect-video w-full overflow-hidden rounded-card border-2 bg-surface-sunken transition-[border-color,box-shadow] " +
            "group-hover/card:border-body/80 group-active/card:border-accent group-focus-visible/card:border-focus group-focus-visible/card:outline-2 group-focus-visible/card:outline-offset-2 group-focus-visible/card:outline-focus motion-reduce:transition-none " +
            (selected ? "border-accent" : "border-transparent")
          }
        >
          {video.thumbnailUrl !== undefined ? (
            <img
              src={video.thumbnailUrl}
              alt=""
              loading="lazy"
              // 復号を別の仕事にする。1 画面に数十枚並ぶので、ここで詰まると
              // スクロールの反応が鈍る（R-408 / SC-008）。
              decoding="async"
              // 動きを減らす設定では遷移だけを止める（FR-023 / R-410）。
              // opacity の最終値は変わらないので、ホバーの手応えは残る。
              className="h-full w-full object-cover transition-opacity duration-200 group-hover/card:opacity-90 group-active/card:opacity-75 motion-reduce:transition-none"
            />
          ) : (
            <span className="flex h-full w-full items-center justify-center px-2 text-center text-xs text-muted">
              {video.thumbnailState === "failed"
                ? "画像を作れませんでした"
                : "画像を準備中"}
            </span>
          )}

          {/*
          時間バッジ（C9）はサムネイルの右下に乗る。地は `--color-badge` で
          **不透明**である（FR-009 / 005 の contracts/design-tokens.md 6.）。
          半透明にすると明るいサムネイルの上で読めなくなる。

          尺が未取得のとき（formatDuration が空を返すとき）は**出さない**
          （spec US3-3）。0:00 と「まだ分からない」を同じ見た目にしない。

          桁は `tabular-nums` で揃える。等幅フォントにはしない — 原案のバッジは
          地の書体のままで、数字の幅だけが揃っていればカードごとに位置が動かない。
        */}
          {duration !== "" && (
            <span className="absolute right-1.5 bottom-1.5 rounded-control bg-badge px-1.5 py-0.5 text-xs font-medium text-body shadow-sm tabular-nums">
              {duration}
            </span>
          )}

          {/* 視聴済みと途中まで見た動画を一覧上で区別できるようにする（FR-006）。
            見終わったものは印で、途中のものは残りの量が分かる帯で示す。 */}
          {watched && (
            <span className="absolute top-1.5 right-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-accent text-accent-ink shadow-sm">
              <Icon name="check" className="h-3.5 w-3.5" />
              <span className="sr-only">視聴済み</span>
            </span>
          )}

          {!watched && watchedRatio !== null && (
            <span
              aria-label={`${String(Math.round(watchedRatio * 100))}% まで再生済み`}
              className="absolute inset-x-0 bottom-0 h-1 bg-surface-sunken"
            >
              <span
                className="block h-full bg-accent"
                style={{ width: `${String(Math.round(watchedRatio * 100))}%` }}
              />
            </span>
          )}

          {unplayable !== null && (
            <span className="absolute top-1.5 left-1.5 rounded-control bg-warning-surface px-1.5 py-0.5 text-xs text-warning">
              {unplayable}
            </span>
          )}
        </div>

        {/* 題名は一行に固定し、カードごとの高さを揃える。全文は title と
            リンクのアクセシブル名から到達できる。 */}
        <div className="pt-2 pb-1">
          <h3
            title={video.title}
            className={
              "truncate rounded-control text-sm leading-5 font-medium transition-colors group-focus-visible/card:text-accent motion-reduce:transition-none " +
              (watched ? "text-muted" : "text-body")
            }
          >
            {video.title}
          </h3>
        </div>
      </Link>
    </article>
  );
}
