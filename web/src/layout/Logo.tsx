import Icon from "./icons";

/**
 * Logo は製品アイコンと製品名を 2 段で並べる（C2 / FR-003）。
 *
 * **`h1` はここが持つ**（R-508）。原案の一覧には見出し文字が無く、製品名を
 * 名乗るのはロゴだけである。見出しを一覧側にも置くと `h1` が 2 つになるので、
 * 「どの幅でも画面に存在する」と決めた要素（FR-003）に持たせる。
 *
 * **リンクにしない・対話要素にしない**（contracts/components.md 1.）。行き先が
 * 既存の `/` であっても、`Tab` の停止位置と操作が 1 つ増えれば利用者にできる
 * ことが増える（FR-013）。素の一覧へ戻る道は NavItem「すべての動画」が既に
 * 持っているので、増やす必要も無い。
 *
 * このロゴは DOM の 2 か所（サイドバーの中とヘッダーの中）に置かれ、使わない
 * 側は `display: none` で消える（contracts/layout.md 3.）。`display: none` は
 * 支援技術からも消えるので、製品名が 2 回読まれることはない。
 */
export default function Logo({ className = "" }: { className?: string }) {
  return (
    <div className={`flex flex-col gap-0.5 ${className}`}>
      <Icon name="film" className="h-5 w-5 text-accent" />
      <h1 className="text-base leading-tight font-semibold tracking-tight text-body">
        vv
      </h1>
    </div>
  );
}
