import { useId } from "react";

import type { TagKind } from "../api/tagKinds";
import { t } from "../i18n";
import { Field, FieldLabel } from "../ui/shadcn/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "../ui/shadcn/select";
import type { TagKindFilter } from "./tagListUrl";

/** anyKind は「Any kind」（絞らない）の `Select` の値である。 */
const anyKind = "any";

/**
 * TagKindSelect は「Filter」の吹き出しの「Kind」の行である。値は `Any kind`、種類の 1 つ、
 * `No kind` で、選ぶとすぐ効く。種類は名前の自然順で、`No kind` は区切り線の後ろに置く。
 * 種類をまだ読んでいない間は `Any kind` と `No kind` だけを並べる。
 */
export default function TagKindSelect({
  kind,
  kinds,
  onKindChange,
}: {
  kind: TagKindFilter;
  /** 名前の自然順の種類。読み込み前は undefined。 */
  kinds: readonly TagKind[] | undefined;
  onKindChange: (kind: TagKindFilter) => void;
}) {
  const id = useId();
  const list = kinds ?? [];
  return (
    <Field>
      <FieldLabel htmlFor={id}>{t.tags.kindFilter.label}</FieldLabel>
      <Select
        value={kind ?? anyKind}
        onValueChange={(value) => onKindChange(value === anyKind ? null : value)}
      >
        <SelectTrigger id={id} size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={anyKind}>{t.tags.kindFilter.any}</SelectItem>
          {list.map((item) => (
            <SelectItem key={item.id} value={String(item.id)}>
              <span title={item.name} className="max-w-chip-label min-w-0 truncate">
                {item.name}
              </span>
            </SelectItem>
          ))}
          <SelectSeparator />
          <SelectItem value="none">{t.tags.kindFilter.none}</SelectItem>
        </SelectContent>
      </Select>
    </Field>
  );
}
