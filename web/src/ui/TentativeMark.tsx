import { CircleDashed } from "lucide-react";

/**
 * TentativeMark は仮のタグ（`tentative: true`）の目印である
 * （specs/031-tentative-tags/ui-design.md「Tentative mark」）。タグの名前の
 * 後ろに `gap-1` で置き、名前が省略されても残るよう `shrink-0` にする。
 * 読み上げは置く側が持つ（押せる要素は読み上げ名に「(tentative)」を添え、
 * 押せない要素は視覚的に隠した「Tentative」を置く）ので、ここは隠す。
 *
 * 大きさはチップでは既定の `size-3`（Folder の目印と同じ）、管理画面の行では
 * `size="row"` の `size-3.5`。
 */
export default function TentativeMark({ size = "chip" }: { size?: "chip" | "row" }) {
  return (
    <CircleDashed
      className={`${size === "chip" ? "size-3" : "size-3.5"} shrink-0 text-fg-subtle`}
      aria-hidden="true"
    />
  );
}
