import type { TagKindRef } from "../api/tagKinds";
import { t } from "../i18n";
import { Badge } from "../ui/shadcn/badge";

/**
 * TagKindLabel はタグの行の名前（と仮の目印）の後ろに置く種類の名札である
 * （specs/051-tag-kinds/ui-design.md「Kind on a row」）。枠だけで塗らない `outline` の
 * `Badge` を一段小さい文字・弱い色にし、名前より先に目に入らず、タグのチップ（塗り）とも
 * 見分けがつくようにする。名前の行の高さを変えないよう `h-5` にし、`sm` 未満は名前の行の
 * 半分、`sm` 以上は `max-w-chip-label` で省略する。押せない。種類の無いタグには何も描かない。
 */
export default function TagKindLabel({ kind }: { kind: TagKindRef | null }) {
  if (kind === null) return null;
  return (
    <Badge
      variant="outline"
      title={t.tags.kindFilter.chip(kind.name)}
      // 名前のリンクの押す範囲（`after:` の疑似要素）より上に置き、`title` を受けられるようにする。
      className="relative h-5 max-w-1/2 font-normal text-muted-foreground sm:max-w-chip-label"
    >
      <span className="sr-only">{t.tags.kindLabel.prefix} </span>
      <span className="min-w-0 truncate">{kind.name}</span>
    </Badge>
  );
}
