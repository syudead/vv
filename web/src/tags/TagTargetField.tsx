import { useMemo, useState } from "react";

import type { Tag } from "../api/tags";
import { t, type UiText } from "../i18n";
import TagCommand, { type TagChoice } from "../ui/TagCommand";
import { MergeTargetField } from "../ui/MergeParts";
import { useTagTargetSearch } from "./tagTargetSearch";

/** The tag the tag page's move or merge acts on, chosen by search; never the page's own tag. */
export function useTagTarget(ownId: number, localOnly = false) {
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Tag | null>(null);
  const excluded = useMemo(() => new Set([ownId]), [ownId]);
  const search = useTagTargetSearch(value, excluded, localOnly);

  function change(next: string) {
    setValue(next);
    setTarget(null);
  }

  function select(option: TagChoice) {
    const found = search.tags.find((item) => String(item.id) === option.id);
    if (found === undefined) return;
    setTarget(found);
    setValue(found.name);
  }

  return { value, target, search, change, select };
}

export type TagTarget = ReturnType<typeof useTagTarget>;

export function TagTargetField({
  label,
  choice,
  layout,
}: {
  label: UiText;
  choice: TagTarget;
  /** See MergeTargetField: `dropdown` on a page, `inline` in a dialog. */
  layout?: "inline" | "dropdown";
}) {
  const { search, target } = choice;
  return (
    <MergeTargetField
      Command={TagCommand}
      layout={layout}
      label={label}
      value={choice.value}
      onValueChange={choice.change}
      choices={search.options}
      exactChoice={search.exactOption}
      onSelect={choice.select}
      chosenId={target === null ? undefined : String(target.id)}
      searching={search.searching}
      searchError={
        search.searchError === null
          ? null
          : t.tags.mergeDialog.searchFailed(search.searchError)
      }
      emptyText={t.tags.mergeDialog.noCandidates}
    />
  );
}
