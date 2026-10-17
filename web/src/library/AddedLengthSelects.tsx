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

/**
 * PresetSelect は追加日か長さの、ラベル付きの `Select` 1 行である。ライブラリの絞り込みの
 * ポップオーバーの `Tag kind` の行と同じ形（`Field`、`h-8` の `Select`、ポップオーバーの幅）。
 */
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

/**
 * AddedSelect は `Added` の行である。
 * 選択肢に無い日数は `Last {N} days` として最後に出し、選んだ形にする。
 */
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

/**
 * LengthSelect は `Length` の行である。選択肢に無い範囲は
 * その範囲の名前で最後に出し、選んだ形にする。
 */
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
