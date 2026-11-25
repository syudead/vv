import type { Ref } from "react";

import { t } from "../../../i18n";
import { LinkRows } from "../../../player/detailsEditor/BasicsPanel";
import { IdRows, Problem, saveOnEnter } from "../../../player/detailsEditor/fields";
import { maxExternalIds } from "../../CreditDetailsDialog";
import { NameRows } from "../../NameRows";
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
  /** Shown in the blank first row; a provisional person saves without a name. */
  namePlaceholder?: string;
}) {
  const onEnter = saveOnEnter(save);
  const rows = { saving, problem, onEnter };
  return (
    <>
      <NameRows
        rows={draft.names}
        onChange={(names) => update((current) => ({ ...current, names }))}
        title={t.credits.names.title}
        firstPlaceholder={namePlaceholder}
        firstRef={nameRef}
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
