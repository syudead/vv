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

// Kind IDs are positive, so this sentinel cannot collide with an actual kind.
const anyValue = "any";

export interface TagKindSelectProps {
  value: number | undefined;
  kinds: readonly TagKind[] | undefined;
  onValueChange: (id: number | undefined) => void;
}

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
        {/* Constrain long kind names to the popup width. */}
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
