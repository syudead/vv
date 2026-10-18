import { useId } from "react";

import { t, type UiText } from "../i18n";
import { Field, FieldLabel } from "../ui/shadcn/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/shadcn/select";
import {
  type AddedLengthOption,
  addedOptions,
  type LengthRange,
  lengthOptions,
} from "./addedLengthCriteria";

function PresetSelect<T>({
  label,
  options,
  value,
  onChoose,
}: {
  label: UiText;
  options: readonly AddedLengthOption<T>[];
  value: string;
  onChoose: (choice: T) => void;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select
        value={value}
        onValueChange={(next) => {
          const option = options.find((entry) => entry.value === next);
          if (option !== undefined) onChoose(option.choice);
        }}
      >
        <SelectTrigger id={id} size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="max-w-popover">
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}

/** Append custom day counts as a selected option. */
export function AddedSelect({
  value,
  onValueChange,
}: {
  value: number | undefined;
  onValueChange: (days: number | undefined) => void;
}) {
  const { options, value: selected } = addedOptions(value);
  return (
    <PresetSelect
      label={t.list.filter.added}
      options={options}
      value={selected}
      onChoose={onValueChange}
    />
  );
}

/** Append custom ranges as a selected option. */
export function LengthSelect({
  value,
  onValueChange,
}: {
  value: LengthRange;
  onValueChange: (range: LengthRange) => void;
}) {
  const { options, value: selected } = lengthOptions(value);
  return (
    <PresetSelect
      label={t.list.filter.length}
      options={options}
      value={selected}
      onChoose={onValueChange}
    />
  );
}
