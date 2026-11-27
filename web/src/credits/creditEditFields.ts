import { RequestFailed } from "../api/client";
import type { CreditPatch, ExternalId } from "../api/credits";
import { errorText } from "../i18n";
import { type IdRow, newRowKey } from "../player/detailsEditor/draft";
import type { FieldProblem } from "../player/detailsEditor/fields";
import { type NameRow, nameRowsControl, nameRowsToSend } from "./NameRows";
import { organizationProfileControl } from "./OrganizationProfileFields";

// Field helpers shared by the credit editors: external ID rows and the mapping of a refused field to its input.

export const maxExternalIds = 20;

/** Map submitted array indexes back to stable row keys for field errors. */
export interface Sent {
  nameKeys: string[];
  idKeys: string[];
}

export function problemOf(failure: unknown, sent: Sent): FieldProblem | null {
  if (!(failure instanceof RequestFailed)) return null;
  const text = errorText(failure);
  if (failure.reason === "organization_parent_cycle" || failure.field === "parentId") {
    return { control: "parent", text };
  }
  const field = failure.field ?? "";
  const profileControl = organizationProfileControl(field);
  if (profileControl !== undefined) return { control: profileControl, text };
  const name = nameRowsControl(field, sent.nameKeys);
  if (name !== undefined) return { control: name, text };
  if (field === "externalIds") return { control: "ids", text };
  const id = /^externalIds\[(\d+)\](?:\.(source|value))?$/.exec(field);
  if (id !== null) {
    const key = sent.idKeys[Number(id[1])];
    if (key !== undefined) return { control: `id-${id[2] ?? "value"}-${key}`, text };
  }
  return null;
}

/** Whether two sent values are equal, for comparing a draft's lists with the loaded ones. */
export function sameJson(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/** Ignore newly added ID rows whose inputs are all blank. */
function blank(...values: string[]): boolean {
  return values.every((value) => value.trim() === "");
}

export function idsToSend(rows: readonly IdRow[]): {
  values: ExternalId[];
  keys: string[];
} {
  const kept = rows.filter((row) => !blank(row.source, row.value));
  return {
    values: kept.map((row) => ({ source: row.source.trim(), value: row.value.trim() })),
    keys: kept.map((row) => row.key),
  };
}

export function idRowsOf(ids: readonly ExternalId[]): IdRow[] {
  return ids.map((id) => ({ key: newRowKey("i"), source: id.source, value: id.value }));
}

/** The rows both kinds' drafts hold. */
export interface NamesAndIds {
  names: NameRow[];
  externalIds: IdRow[];
}

/** `names` and `externalIds` of a PATCH, each only when it differs from the loaded rows. */
export function namesAndIdsPatch(
  draft: NamesAndIds,
  loaded: NamesAndIds,
): { patch: CreditPatch; sent: Sent } {
  const patch: CreditPatch = {};
  const names = nameRowsToSend(draft.names);
  if (!sameJson(names.values, nameRowsToSend(loaded.names).values))
    patch.names = names.values;
  const ids = idsToSend(draft.externalIds);
  if (!sameJson(ids.values, idsToSend(loaded.externalIds).values))
    patch.externalIds = ids.values;
  return { patch, sent: { nameKeys: names.keys, idKeys: ids.keys } };
}
