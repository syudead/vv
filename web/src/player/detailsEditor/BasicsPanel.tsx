import type { Ref } from "react";

import { type WorkDateKind, workDateKinds } from "../../api/workInfo";
import { t, type UiText } from "../../i18n";
import { Field, FieldDescription, FieldLabel, FieldTitle } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import { Textarea } from "../../ui/shadcn/textarea";
import { type Draft, type LinkRow, newRowKey } from "./draft";
import {
  AddRow,
  controlId,
  type FieldProblem,
  IdRows,
  invalidProps,
  Problem,
  RemoveRow,
  type RowsProps,
  saveOnEnter,
  useFocusAdded,
} from "./fields";

function dateLabel(kind: WorkDateKind): UiText {
  const e = t.player.detailsEditor;
  if (kind === "release") return e.released;
  if (kind === "recorded") return e.recorded;
  return e.published;
}

export interface PanelProps {
  draft: Draft;
  update: (change: (draft: Draft) => Draft) => void;
  /** Keep inputs read-only while saving. */
  saving: boolean;
  problem: FieldProblem | null;
  save: () => void;
}

export default function BasicsPanel({
  draft,
  update,
  saving,
  problem,
  save,
  namePlaceholder,
  nameRef,
}: PanelProps & {
  namePlaceholder: string;
  nameRef: Ref<HTMLInputElement>;
}) {
  const e = t.player.detailsEditor;
  const onEnter = saveOnEnter(save);
  return (
    <>
      <Field>
        <FieldLabel htmlFor={controlId("name")}>{e.name}</FieldLabel>
        <Input
          ref={nameRef}
          id={controlId("name")}
          value={draft.name}
          placeholder={namePlaceholder}
          readOnly={saving}
          onChange={(event) => {
            const name = event.target.value;
            update((current) => ({ ...current, name }));
          }}
          onKeyDown={onEnter}
          className="h-8"
          {...invalidProps(problem, "name")}
        />
        <Problem problem={problem} control="name" />
      </Field>
      <div className="flex flex-col gap-2">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor={controlId("code")}>{e.code}</FieldLabel>
            <Input
              id={controlId("code")}
              value={draft.code}
              readOnly={saving}
              onChange={(event) => {
                const code = event.target.value;
                update((current) => ({ ...current, code }));
              }}
              onKeyDown={onEnter}
              className="h-8 tabular-nums"
              {...invalidProps(problem, "code")}
            />
            <Problem problem={problem} control="code" />
          </Field>
          {workDateKinds.map((kind) => (
            <Field key={kind}>
              <FieldLabel htmlFor={controlId(`date-${kind}`)}>
                {dateLabel(kind)}
              </FieldLabel>
              <Input
                id={controlId(`date-${kind}`)}
                value={draft.dates[kind]}
                placeholder={e.datePlaceholder}
                readOnly={saving}
                onChange={(event) => {
                  const date = event.target.value;
                  update((current) => ({
                    ...current,
                    dates: { ...current.dates, [kind]: date },
                  }));
                }}
                onKeyDown={onEnter}
                className="h-8 tabular-nums"
                {...invalidProps(problem, `date-${kind}`)}
              />
              <Problem problem={problem} control={`date-${kind}`} />
            </Field>
          ))}
        </div>
        <FieldDescription>{e.dateHint}</FieldDescription>
      </div>
      <Field className="flex-1">
        <FieldLabel htmlFor={controlId("description")}>{e.description}</FieldLabel>
        <Textarea
          id={controlId("description")}
          value={draft.description}
          rows={4}
          readOnly={saving}
          onChange={(event) => {
            const description = event.target.value;
            update((current) => ({ ...current, description }));
          }}
          className="flex-1 resize-none field-sizing-fixed"
          {...invalidProps(problem, "description")}
        />
        <Problem problem={problem} control="description" />
      </Field>
      <LinkRows
        rows={draft.urls}
        onChange={(urls) => update((current) => ({ ...current, urls }))}
        saving={saving}
        problem={problem}
        onEnter={onEnter}
      />
      <IdRows
        rows={draft.externalIds}
        onChange={(externalIds) => update((current) => ({ ...current, externalIds }))}
        saving={saving}
        problem={problem}
        onEnter={onEnter}
      />
    </>
  );
}

function LinkRows({ rows, onChange, saving, problem, onEnter }: RowsProps<LinkRow>) {
  const e = t.player.detailsEditor;
  const focusAdded = useFocusAdded();
  return (
    <Field aria-labelledby={controlId("links")}>
      <FieldTitle id={controlId("links")}>{e.links}</FieldTitle>
      {rows.map((row, index) => (
        <div key={row.key} className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <Input
              id={controlId(`url-${row.key}`)}
              aria-label={e.link}
              value={row.url}
              readOnly={saving}
              onChange={(event) =>
                onChange(
                  rows.map((item, at) =>
                    at === index ? { ...item, url: event.target.value } : item,
                  ),
                )
              }
              onKeyDown={onEnter}
              className="h-8"
              {...invalidProps(problem, `url-${row.key}`)}
            />
            <RemoveRow
              label={e.removeLink}
              saving={saving}
              onRemove={() => onChange(rows.filter((_, at) => at !== index))}
            />
          </div>
          <Problem problem={problem} control={`url-${row.key}`} />
        </div>
      ))}
      <AddRow
        label={e.addLink}
        saving={saving}
        onAdd={() => {
          const key = newRowKey("u");
          focusAdded(`url-${key}`);
          onChange([...rows, { key, url: "" }]);
        }}
      />
    </Field>
  );
}
