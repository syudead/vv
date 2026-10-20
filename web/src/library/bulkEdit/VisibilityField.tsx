import { useId } from "react";

import { t } from "../../i18n";
import { Field, FieldDescription, FieldLabel } from "../../ui/shadcn/field";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";

type Value = "keep" | "public" | "private";

/** How many targets are public now, and Keep, Public or Private for all of them. */
export default function VisibilityField({
  total,
  publicCount,
  value,
  disabled,
  onChange,
}: {
  total: number;
  publicCount: number;
  value: boolean | undefined;
  disabled: boolean;
  onChange: (value: boolean | undefined) => void;
}) {
  const b = t.library.bulkEdit;
  const labelId = useId();
  const current: Value = value === undefined ? "keep" : value ? "public" : "private";
  return (
    <Field>
      <FieldLabel id={labelId}>{b.visibility}</FieldLabel>
      <FieldDescription>{b.publicNow(publicCount, total)}</FieldDescription>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        className="w-fit"
        aria-labelledby={labelId}
        disabled={disabled}
        value={current}
        onValueChange={(next) => {
          if (next === "") return;
          onChange(next === "keep" ? undefined : next === "public");
        }}
      >
        <ToggleGroupItem value="keep">{b.keep}</ToggleGroupItem>
        <ToggleGroupItem value="public">{b.public}</ToggleGroupItem>
        <ToggleGroupItem value="private">{b.private}</ToggleGroupItem>
      </ToggleGroup>
    </Field>
  );
}
