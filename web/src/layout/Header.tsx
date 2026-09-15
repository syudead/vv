import Logo from "./Logo";

/**
 * Header は上に固定される横の帯である（C5 / FR-001・FR-002）。
 *
 * 左端はサイドバーの右（幅 640px 未満では画面の左端）で、スクロールしない。
 * サイドバーと同じく `position: fixed` である（R-501）。
 *
 * **ロゴをもう 1 つ置く。** 幅 640px 以上ではサイドバーの側が本体なので、
 * ここは `sm:hidden` で消す（contracts/layout.md 3.）。DOM に 2 か所ある理由は
 * FR-003 が「どの幅でも画面上に存在する」ことと「640px 未満ではヘッダーへ
 * 移す」ことを同時に求めているためで、`display: none` は支援技術からも消える
 * ので製品名が 2 回読まれることはない（R-502）。
 *
 * 幅の分岐は CSS だけで閉じる。`matchMedia` で幅を監視しない。
 *
 * 中身（タブと設定）は US2 で入る。
 */
export default function Header() {
  return (
    <header
      data-app-header=""
      className={
        "fixed top-0 right-0 left-0 z-30 h-[var(--size-header)] " +
        "border-b border-border bg-surface sm:left-[var(--size-sidebar)]"
      }
    >
      <div className="flex h-full items-center px-4">
        <Logo className="sm:hidden" />
      </div>
    </header>
  );
}

/**
 * headerHeight は固定ヘッダーの**実測**の高さ（px）である。
 *
 * `--size-header` の値を JavaScript 側へ書き写さないために、描かれた要素を
 * 測る。写すと、トークンを変えたときにここだけ古い値が残る。
 *
 * 骨格を通さない画面（`/videos/:id`。FR-015）や、骨格を持たずに描くテストでは
 * ヘッダーが存在しないので 0 を返す ── 隠れる相手が無いのだから、逃げる高さも
 * 0 でよい。
 */
export function headerHeight(): number {
  const header = document.querySelector("[data-app-header]");
  return header?.getBoundingClientRect().height ?? 0;
}
