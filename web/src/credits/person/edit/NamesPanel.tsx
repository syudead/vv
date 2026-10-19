import type { Ref } from "react";

import { t } from "../../../i18n";
import { LinkRows } from "../../../player/detailsEditor/BasicsPanel";
import {
  IdRows,
  NameField,
  Problem,
  saveOnEnter,
} from "../../../player/detailsEditor/fields";
import { AliasRows, maxExternalIds } from "../../CreditDetailsDialog";
import type { PersonPanelProps } from "./ProfilePanel";

const text = t.credits.personEdit;

export default function NamesPanel({
  draft,
  update,
  saving,
  problem,
  save,
  nameRef,
  namePlaceholder,
}: PersonPanelProps & {
  nameRef: Ref<HTMLInputElement>;
  /** Shown while the name is empty; a provisional person saves without one. */
  namePlaceholder?: string;
}) {
  const onEnter = saveOnEnter(save);
  const rows = { saving, problem, onEnter };
  return (
    <>
      <NameField
        label={text.name}
        value={draft.name}
        placeholder={namePlaceholder}
        onChange={(name) => update((current) => ({ ...current, name }))}
        nameRef={nameRef}
        {...rows}
      />
      <AliasRows
        rows={draft.aliases}
        onChange={(aliases) => update((current) => ({ ...current, aliases }))}
        title={text.aliases}
        {...rows}
      />
      <div className="flex flex-col gap-1">
        <IdRows
          rows={draft.externalIds}
          onChange={(externalIds) => update((current) => ({ ...current, externalIds }))}
          title={text.externalIds}
          max={maxExternalIds}
          {...rows}
        />
        <Problem problem={problem} control="ids" />
      </div>
      <div className="flex flex-col gap-1">
        <LinkRows
          rows={draft.links}
          onChange={(links) => update((current) => ({ ...current, links }))}
          rowLabel={text.url}
          {...rows}
        />
        <Problem problem={problem} control="links" />
      </div>
    </>
  );
}
