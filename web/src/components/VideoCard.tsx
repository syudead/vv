import { Link } from "react-router";

import type { Video } from "../api/client";

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
  "hover:after:bg-body/10 active:after:bg-surface-sunken/60 " +
  "motion-reduce:after:transition-none " +
  "outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus";

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
 * 題名の全文への到達手段は 3 つある（R-409 / FR-011）。
 *
 * 1. リンクのアクセシブル名は**常に全文**である（aria-label）。見た目の省略に
 *    引きずられない。
 * 2. ポインタには title 属性で全文を出す。
 * 3. キーボードで狙いを合わせると省略が解ける。
 *
 * 3 の解けた題名は**枠の上に重ねて**描く。題名の場所を広げると、長い題名の
 * 項目だけ背が高くなって格子が崩れる（FR-005 / SC-003）。
 */
export default function VideoCard({
  video,
  backTo,
}: {
  video: Video;
  /**
   * backTo は遷移元の一覧の URL である（FR-016）。再生画面の「一覧へ戻る」が
   * ここへ帰る。渡さないと再生画面は `/` へ戻すので、検索語と並び順が消える。
   */
  backTo?: string;
}) {
  const duration = formatDuration(video.durationMs);
  const unplayable = unplayableText(video);
  const watched = video.progress?.completed === true;
  const watchedRatio = partialRatio(video);

  return (
    <Link
      to={`/videos/${String(video.id)}`}
      // 帰り道を持たせる。URL のクエリ（検索語・並び順）は一覧側にしか無いので、
      // ここで渡さないと再生画面は「どの一覧から来たか」を知りようがない。
      state={backTo === undefined ? undefined : { from: backTo }}
      // 見た目は省略しても、読み上げには必ず全文を渡す（R-409）。名前を
      // 中身から組み立てると、枠の中の小片（長さ・視聴済み）まで名前に
      // 混ざってしまう。
      aria-label={video.title}
      className={"group flex flex-col rounded-card bg-surface-raised " + states}
    >
      <div className="relative aspect-video w-full overflow-hidden rounded-t-card bg-surface-sunken">
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
            className="h-full w-full object-cover transition-opacity group-hover:opacity-90 motion-reduce:transition-none"
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
          <span className="absolute right-1.5 bottom-1.5 rounded-control bg-badge px-1.5 py-0.5 text-xs text-body tabular-nums">
            {duration}
          </span>
        )}

        {/* 視聴済みと途中まで見た動画を一覧上で区別できるようにする（FR-006）。
            見終わったものは印で、途中のものは残りの量が分かる帯で示す。 */}
        {watched && (
          <span className="absolute top-1.5 right-1.5 rounded-control bg-accent px-1.5 py-0.5 text-xs text-accent-ink">
            視聴済み
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

      {/*
        題名の場所は 2 行分で固定する（h-10）。中の題名は下端を揃えて重ね置き
        してあるので、狙いを合わせて省略が解けると**上へ**伸び、枠の上に
        重なる。項目の高さは変わらない。

        余白は外側の器が持つ。題名は `inset-x-0` で置かれるが、絶対配置の基準は
        器の**パディングの内側**なので、省略が解けて伸びたときも枠の縁に触れない。
      */}
      <div className="px-2 pt-1.5 pb-2">
        <div className="relative h-10">
          <h3
            title={video.title}
            className={
              "absolute inset-x-0 bottom-0 line-clamp-2 rounded-control text-sm leading-snug font-medium group-hover:underline " +
              "group-focus-visible:line-clamp-none group-focus-visible:bg-surface-raised group-focus-visible:p-1 " +
              (watched ? "text-muted" : "text-body")
            }
          >
            {video.title}
          </h3>
        </div>
      </div>
    </Link>
  );
}
