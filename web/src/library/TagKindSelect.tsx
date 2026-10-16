import { useId, useMemo } from "react";

import { sortTagKinds, type TagKind } from "../api/tagKinds";
import { t } from "../i18n";
import { Field, FieldLabel } from "../ui/shadcn/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/shadcn/select";

/** anyValue は `Any kind` の選択肢の値である（種類の id は正の整数なので重ならない）。 */
const anyValue = "any";

export interface TagKindSelectProps {
  /** 絞り込み中の種類の id。undefined は `Any kind`。 */
  value: number | undefined;
  /** 共有の種類の一覧。届く前は undefined で、`Any kind` だけを並べる。 */
  kinds: readonly TagKind[] | undefined;
  onValueChange: (id: number | undefined) => void;
}

/**
 * TagKindSelect はライブラリの絞り込みのポップオーバーの最後の行、`Tag kind` の `Select` で
 * ある（specs/051-tag-kinds/ui-design.md「Library」の Kind filter）。選択肢は `Any kind` と
 * 名前の自然順の種類で、`No kind` は無い（research.md R-5）。選ぶとすぐ効く。
 */
export default function TagKindSelect({
  value,
  kinds,
  onValueChange,
}: TagKindSelectProps) {
  const id = useId();
  const sorted = useMemo(() => sortTagKinds(kinds ?? []), [kinds]);
  return (
    <Field>
      <FieldLabel htmlFor={id}>{t.library.tagKindFilter.label}</FieldLabel>
      <Select
        value={value === undefined ? anyValue : String(value)}
        onValueChange={(next) =>
          onValueChange(next === anyValue ? undefined : Number(next))
        }
      >
        <SelectTrigger id={id} size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        {/* 長い種類の名前で画面の横にはみ出さないよう、一覧はポップオーバーの幅で切る。 */}
        <SelectContent className="max-w-popover">
          <SelectItem value={anyValue}>{t.library.tagKindFilter.any}</SelectItem>
          {sorted.map((kind) => (
            <SelectItem
              key={kind.id}
              value={String(kind.id)}
              title={kind.name}
              className="*:[span]:last:min-w-0"
            >
              <span className="truncate">{kind.name}</span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}
