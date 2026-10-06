/**
 * factTrigger は、情報の行（VideoFacts）の項目の位置に置き、押すと吹き出しを開く引き金
 * （日付・バージョンの本数）に `Button` の `ghost` の変種と一緒に渡すクラスである。見た目は
 * アイコンと値だけの他の項目と同じにし、引き金であることは hover の地だけで示す。行の高さを
 * 変えないよう、ボタンの高さを外し、地が文字の外へ少しだけ出るよう左右に余白を取ってその分を
 * 外へずらす。
 */
export const factTrigger =
  "-mx-1 h-auto gap-1.5 rounded-sm px-1 py-0 font-normal text-muted-foreground has-[>svg]:px-1";
