/**
 * factTrigger は、ファイルの情報（FactList）の値の位置に置き、押すと吹き出しを開く引き金
 * （日付・バージョンの本数）に `Button` の `link` の変種と一緒に渡すクラスである。値は文の
 * 中のリンクのように読ませ、行の高さを変えないよう、ボタンの高さと左右の余白を外す
 * （web/registry/rules/components.md「Button」の `link`）。
 */
export const factTrigger = "h-auto gap-1 px-0 py-0 font-normal has-[>svg]:px-0";
