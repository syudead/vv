import { Heart } from "lucide-react";

import { t } from "../../i18n";
import { Toggle } from "../../ui/shadcn/toggle";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
import { type NameFilter, nameFilters } from "./peopleCriteria";

const words = t.credits.peopleGrid.nameFilter;

function isNameFilter(value: string): value is NameFilter {
  return (nameFilters as readonly string[]).includes(value);
}

/** The grid's filters stay in view above it, so the applied ones read at a glance. */
export default function PeopleFilters({
  named,
  favorite,
  disabled,
  onNamedChange,
  onFavoriteChange,
}: {
  named: NameFilter;
  favorite: boolean;
  disabled: boolean;
  onNamedChange: (value: NameFilter) => void;
  onFavoriteChange: (value: boolean) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        aria-label={words.label}
        value={named}
        disabled={disabled}
        onValueChange={(next) => {
          // One option always stays selected.
          if (isNameFilter(next) && next !== named) onNamedChange(next);
        }}
      >
        {nameFilters.map((option) => (
          <ToggleGroupItem key={option} value={option} className="flex-none px-3">
            {words[option]}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <Toggle
        variant="outline"
        size="sm"
        className="px-3"
        pressed={favorite}
        disabled={disabled}
        onPressedChange={onFavoriteChange}
      >
        <Heart aria-hidden="true" />
        {t.credits.peopleGrid.favorites}
      </Toggle>
    </div>
  );
}
