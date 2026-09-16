import type { ReactNode } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import Icon from "../layout/icons";
import MetaList from "./MetaList";

/**
 * states は C15 の 4 状態である（contracts/components.md 2.）。
 *
 * NavItem と同じく hover / active を `::after` の覆いで表す。パネルの地の上でも
 * 映像の脇でも同じ向きに 1 段動き、`isolate` + `-z-10` で覆いを中身の下へ敷くので
 * × の線は覆われない。動きを減らす設定では遷移だけを止め、**色の最終状態は常に
 * 適用する**（FR-012。操作の結果は判別できる）。
 */
const states =
  "relative isolate after:pointer-events-none after:absolute after:inset-0 " +
  "after:-z-10 after:rounded-control after:transition-colors " +
  "hover:after:bg-body/10 active:after:bg-surface-sunken/60 " +
  "motion-reduce:after:transition-none " +
  "outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus";

/**
 * CloseButton はパネル右上の × である（C15 / FR-017）。
 *
 * **映像に重ねない。** 重ねると映像の一部を隠し、操作列と当たり判定が競合する。
 * パネルの中の最初の行に置けば、どの幅でも映像の外にある
 * （contracts/layout.md 4.）。
 *
 * 行き先は遷移元の一覧で、**スクロール位置の復元は 004 の振る舞いのまま**である
 * （`Link` が履歴を 1 つ進め、一覧側が控えから位置を戻す）。`<button>` +
 * `navigate()` にしないのは、行き先のあるものをリンクにしておくと新しいタブで
 * 開く・戻るといったブラウザの作法がそのまま効くためである。
 *
 * 読み上げ用の名前を「一覧へ戻る」にするのは、× という字面だけでは行き先が
 * 伝わらないからである。
 */
function CloseButton({ to }: { to: string }) {
  return (
    <Link
      to={to}
      aria-label="一覧へ戻る"
      className={
        `${states} inline-flex min-h-[var(--size-tap)] min-w-[var(--size-tap)] ` +
        "items-center justify-center rounded-control text-muted"
      }
    >
      <Icon name="close" className="h-5 w-5" />
    </Link>
  );
}

/**
 * InfoPanel は再生画面の右（狭い画面では下）に立つ情報パネルである
 * （C14 / FR-015・FR-017 / contracts/layout.md 4.）。
 *
 * 中身の順序は **×（右上）→ 題名 → 知らせ → 動画の情報**に固定する。順序を
 * 呼び出し側に委ねないのは、これが契約だからである。
 *
 * **題名は `h1` のままここに置く。** spec US5-1 が消すと言っているのは「画面の
 * 見出し文字」（ロゴや画面名）であって、US5-3 が求めるパネル内の題名ではない。
 * 再生画面には Logo が無いので（R-505）、ここで `h1` を落とすと画面に見出しが
 * 1 つも無くなる。
 *
 * 幅は器の側（VideoPage の格子）が `minmax(18rem, 24rem)` で与える。ここで
 * 固定幅を持たないのは、幅 640〜800px で映像が極端に細くなるのを避けるため
 * である（R-505）。
 *
 * 取得中・取得に失敗した状態でも、**× だけは必ず出す**。戻れない画面に閉じ込め
 * られると、利用者に残る手が再読み込みしかなくなる。そのとき `title` と `video`
 * は `undefined` で渡る。
 */
export default function InfoPanel({
  backTo,
  title,
  video,
  children,
}: {
  /** backTo は × の行き先である（遷移元の一覧）。 */
  backTo: string;
  /** title は動画の題名。取得できていないうちは `undefined`。 */
  title?: string;
  /** video は動画の情報（C16）。取得できていないうちは `undefined`。 */
  video?: Video;
  /** children は知らせ（再開・再生できない形式・再生の失敗）である。 */
  children?: ReactNode;
}) {
  return (
    <aside className="flex min-w-0 flex-col gap-4 rounded-card bg-surface-raised p-4">
      {/* × は行を独り占めして右端に立つ。題名と同じ行に並べると、長い題名が
          折り返したときに × の位置が下へずれる（spec US5-3 は「右上」と
          言っている）。 */}
      <div className="flex justify-end">
        <CloseButton to={backTo} />
      </div>

      {/* 題名。break-words が要るのは、題名がファイル名由来で、空白の無い長い
          1 語になりうるからである。折り返せない語は狭いパネルでそのまま横
          スクロールになる。 */}
      {title !== undefined && (
        <h1 className="text-lg font-semibold tracking-tight break-words">{title}</h1>
      )}

      {children}

      {video !== undefined && <MetaList video={video} />}
    </aside>
  );
}
