import { useId } from "react";

import { t } from "../../i18n";
import { Field, FieldLabel } from "../../ui/shadcn/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/shadcn/select";
import { type NameFilter, nameFilters } from "./peopleCriteria";

const words = t.credits.peopleGrid.nameFilter;

function isNameFilter(value: string): value is NameFilter {
  return (nameFilters as readonly string[]).includes(value);
}

/** Keeps every person, only named ones or only provisional ones. */
export default function NameFilterSelect({
  value,
  onChange,
}: {
  value: NameFilter;
  onChange: (value: NameFilter) => void;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{words.label}</FieldLabel>
      <Select
        value={value}
        onValueChange={(next) => {
          if (isNameFilter(next)) onChange(next);
        }}
      >
        <SelectTrigger id={id} size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {nameFilters.map((option) => (
            <SelectItem key={option} value={option}>
              {words[option]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}
