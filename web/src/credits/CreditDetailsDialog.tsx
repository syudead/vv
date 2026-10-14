import { useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "../api/client";
import {
  type Alias,
  type Credit,
  type CreditKind,
  type CreditPatch,
  type ExternalId,
  isCreditGone,
  listCredits,
  updateCredit,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { type IdRow, newRowKey } from "../player/detailsEditor/draft";
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
} from "../player/detailsEditor/fields";
import { DialogError } from "../tags/DialogError";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Combobox, type ComboboxOption } from "../ui/shadcn/combobox";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLegend,
  FieldSet,
} from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { parentOf } from "./creditRows";

/** maxAliases・maxExternalIds は 1 件が持てる数である（data-model.md「Fields」）。 */
const maxAliases = 50;
const maxExternalIds = 20;

/** noParent は親の団体の「None」の値である（Combobox の値は空にできない）。 */
const noParent = "none";

/** AliasRow は別名の 1 行の値である。 */
interface AliasRow {
  key: string;
  name: string;
  language: string;
}

/** Sent は送った一覧の位置から、その行の鍵への対応である（`field` を行へ戻す）。 */
interface Sent {
  aliasKeys: string[];
  idKeys: string[];
}

/**
 * problemOf は保存の失敗を、出す場所（`control`）と文に分ける。`field` が行の入力を指せば
 * その入力の下、親の団体（`parentId` と親の輪）なら親の欄の下、一覧の数の上限なら欄の
 * 下に出す。どこも指さなければ null（窓の失敗の 1 行に出す）。
 */
function problemOf(failure: unknown, sent: Sent): FieldProblem | null {
  if (!(failure instanceof RequestFailed)) return null;
  const text = errorText(failure);
  if (failure.reason === "organization_parent_cycle" || failure.field === "parentId") {
    return { control: "parent", text };
  }
  const field = failure.field ?? "";
  if (field === "aliases") return { control: "aliases", text };
  if (field === "externalIds") return { control: "ids", text };
  const alias = /^aliases\[(\d+)\]\.(name|language)$/.exec(field);
  if (alias !== null) {
    const key = sent.aliasKeys[Number(alias[1])];
    if (key !== undefined) return { control: `alias-${alias[2]!}-${key}`, text };
  }
  const id = /^externalIds\[(\d+)\](?:\.(source|value))?$/.exec(field);
  if (id !== null) {
    const key = sent.idKeys[Number(id[1])];
    if (key !== undefined) return { control: `id-${id[2] ?? "value"}-${key}`, text };
  }
  return null;
}

/** Edited は窓を開いてから手を入れた欄である。手を入れていない欄は送らない。 */
interface Edited {
  aliases: boolean;
  ids: boolean;
  parent: boolean;
}

/** blank は行の入力がすべて空白だけかである。そうした行は送らない（足しただけの行）。 */
function blank(...values: string[]): boolean {
  return values.every((value) => value.trim() === "");
}

/**
 * buildPatch は窓の値から `PATCH` の本文を作る。手を入れた欄（`edited`）だけを送り、送る
 * 一覧は丸ごと送る（research.md R-8）。手を入れていない欄を開いたときの値で送ると、そのあと
 * ほかのタブで変えた値を消すため送らない（サーバーは省いた欄を残す）。空の行は送らず、
 * 言語が空の別名は言語を省く。親は団体のときだけ送り、「None」は `null`。
 */
function buildPatch(
  kind: CreditKind,
  aliases: readonly AliasRow[],
  ids: readonly IdRow[],
  parent: string,
  edited: Edited,
): { patch: CreditPatch; sent: Sent } {
  const aliasRows = edited.aliases
    ? aliases.filter((row) => !blank(row.name, row.language))
    : [];
  const idRows = edited.ids ? ids.filter((row) => !blank(row.source, row.value)) : [];
  const sentAliases: Alias[] = aliasRows.map((row) => {
    const language = row.language.trim();
    return language === ""
      ? { name: row.name.trim() }
      : { name: row.name.trim(), language };
  });
  const sentIds: ExternalId[] = idRows.map((row) => ({
    source: row.source.trim(),
    value: row.value.trim(),
  }));
  const patch: CreditPatch = {};
  if (edited.aliases) patch.aliases = sentAliases;
  if (edited.ids) patch.externalIds = sentIds;
  if (kind === "organization" && edited.parent)
    patch.parentId = parent === noParent ? null : Number(parent);
  return {
    patch,
    sent: {
      aliasKeys: aliasRows.map((row) => row.key),
      idKeys: idRows.map((row) => row.key),
    },
  };
}

/**
 * CreditDetailsDialog は人物・団体の詳細の窓 `Edit "{name}"` である（specs/047-video-work-info/
 * ui-design.md「Details dialog」）。別名（言語つき、50 件まで）、外部 ID（20 件まで）、団体
 * だけ親の団体を持ち、1 回の「Save」で手を入れた一覧を丸ごと `PATCH` する。名前はここに
 * 無い（改名は行で行う）。
 *
 * `field` つきの `400` と `409 external_id_taken` はその行の入力の下か親の欄の下に出し、
 * 窓は開いたまま。対象がもう無ければ `onStale`。
 */
export default function CreditDetailsDialog({
  kind,
  credit,
  onClose,
  onSaved,
  onStale,
}: {
  kind: CreditKind;
  credit: Credit;
  onClose: () => void;
  onSaved: (updated: Credit) => void;
  onStale: () => void;
}) {
  const d = t.credits.details;
  const [aliases, setAliases] = useState<AliasRow[]>(() =>
    credit.aliases.map((alias) => ({
      key: newRowKey("a"),
      name: alias.name,
      language: alias.language ?? "",
    })),
  );
  const [ids, setIds] = useState<IdRow[]>(() =>
    credit.externalIds.map((id) => ({
      key: newRowKey("i"),
      source: id.source,
      value: id.value,
    })),
  );
  const [parent, setParent] = useState(() => {
    const current = parentOf(credit);
    return current === undefined ? noParent : String(current.id);
  });
  const [edited, setEdited] = useState<Edited>({
    aliases: false,
    ids: false,
    parent: false,
  });
  const [pending, setPending] = useState(false);
  const [problem, setProblem] = useState<FieldProblem | null>(null);
  const [error, setError] = useState<UiText | null>(null);
  const submitRef = useRef<HTMLButtonElement>(null);

  function save() {
    void submit();
  }

  async function submit() {
    if (pending) return;
    const { patch, sent } = buildPatch(kind, aliases, ids, parent, edited);
    setPending(true);
    setProblem(null);
    setError(null);
    try {
      onSaved(await updateCredit(kind, credit.id, patch));
    } catch (failure) {
      if (isCreditGone(failure)) {
        onStale();
        return;
      }
      const placed = problemOf(failure, sent);
      if (placed === null) setError(errorText(failure));
      else setProblem(placed);
      setPending(false);
    }
  }

  const onEnter = saveOnEnter(save);
  const rowsProps = { saving: pending, problem, onEnter };
  const markEdited = (field: keyof Edited) =>
    setEdited((current) => (current[field] ? current : { ...current, [field]: true }));

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={d.title(credit.name)}
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? d.submitting : d.submit}
        </>
      }
      pending={pending}
      submitRef={submitRef}
    >
      <FieldSet>
        <FieldLegend>{d.aliases}</FieldLegend>
        <AliasRows
          rows={aliases}
          onChange={(next) => {
            setAliases(next);
            markEdited("aliases");
          }}
          {...rowsProps}
        />
      </FieldSet>
      <FieldSet>
        <FieldLegend>{d.externalIds}</FieldLegend>
        <IdRows
          rows={ids}
          onChange={(next) => {
            setIds(next);
            markEdited("ids");
          }}
          title={null}
          max={maxExternalIds}
          {...rowsProps}
        />
        <Problem problem={problem} control="ids" />
      </FieldSet>
      {kind === "organization" && (
        <FieldSet>
          <FieldLegend>{d.parent}</FieldLegend>
          <ParentField
            credit={credit}
            value={parent}
            onChange={(next) => {
              setParent(next);
              markEdited("parent");
              if (problem?.control === "parent") setProblem(null);
            }}
            disabled={pending}
            problem={problem}
          />
        </FieldSet>
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}

/**
 * AliasRows は別名の行（`Alias` と `Language` と ×）と「Add alias」である。`sm` 未満では
 * 2 つの入力を縦に重ねる（ui-design.md「Details dialog」）。
 */
function AliasRows({ rows, onChange, saving, problem, onEnter }: RowsProps<AliasRow>) {
  const d = t.credits.details;
  const focusAdded = useFocusAdded();
  const change = (index: number, patch: Partial<AliasRow>) =>
    onChange(rows.map((item, at) => (at === index ? { ...item, ...patch } : item)));
  return (
    <Field>
      <FieldDescription>{d.languageDescription}</FieldDescription>
      {rows.map((row, index) => (
        <div key={row.key} className="flex flex-col gap-1">
          <div className="flex items-start gap-2">
            <div className="grid min-w-0 flex-1 gap-2 sm:grid-cols-3">
              <Input
                id={controlId(`alias-name-${row.key}`)}
                aria-label={d.alias}
                value={row.name}
                readOnly={saving}
                onChange={(event) => change(index, { name: event.target.value })}
                onKeyDown={onEnter}
                className="h-8 sm:col-span-2"
                {...invalidProps(problem, `alias-name-${row.key}`)}
              />
              <Input
                id={controlId(`alias-language-${row.key}`)}
                aria-label={d.language}
                placeholder={d.languagePlaceholder}
                value={row.language}
                readOnly={saving}
                onChange={(event) => change(index, { language: event.target.value })}
                onKeyDown={onEnter}
                className="h-8"
                {...invalidProps(problem, `alias-language-${row.key}`)}
              />
            </div>
            <RemoveRow
              label={d.removeAlias}
              saving={saving}
              onRemove={() => onChange(rows.filter((_, at) => at !== index))}
            />
          </div>
          <Problem problem={problem} control={`alias-name-${row.key}`} />
          <Problem problem={problem} control={`alias-language-${row.key}`} />
        </div>
      ))}
      <AddRow
        label={d.addAlias}
        saving={saving}
        disabled={rows.length >= maxAliases}
        onAdd={() => {
          const key = newRowKey("a");
          focusAdded(`alias-name-${key}`);
          onChange([...rows, { key, name: "", language: "" }]);
        }}
      />
      <Problem problem={problem} control="aliases" />
    </Field>
  );
}

/**
 * ParentField は団体の親を選ぶ registry の `Combobox` である。候補は窓を開いたときに
 * `GET /api/organizations`（`limit` なし）で読む、この団体のほかのすべての団体で、先頭に
 * 「None」を置く。読み終えるまでは今の親だけを候補にして押せなくする。
 */
function ParentField({
  credit,
  value,
  onChange,
  disabled,
  problem,
}: {
  credit: Credit;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  problem: FieldProblem | null;
}) {
  const d = t.credits.details;
  const [organizations, setOrganizations] = useState<readonly Credit[] | null>(null);
  const [failure, setFailure] = useState<UiText | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    listCredits("organization", {}, controller.signal).then(
      (page) => {
        if (!controller.signal.aborted) setOrganizations(page.items);
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        setFailure(d.parentsFailed(errorText(error)));
      },
    );
    return () => controller.abort();
  }, [d]);

  const current = parentOf(credit);
  const others =
    organizations ??
    (current === undefined ? [] : [{ id: current.id, name: current.name }]);
  const options: ComboboxOption[] = [
    { value: noParent, label: d.parentNone },
    ...others
      .filter((organization) => organization.id !== credit.id)
      .map((organization) => ({
        value: String(organization.id),
        label: organization.name,
      })),
  ];

  return (
    <Field data-invalid={problem?.control === "parent" || undefined}>
      <div id={controlId("parent")}>
        <Combobox
          options={options}
          value={value}
          onValueChange={onChange}
          placeholder={d.parentNone}
          searchPlaceholder={d.parentSearch}
          emptyText={d.parentEmpty}
          disabled={disabled || organizations === null}
          aria-label={d.parent}
          className="w-full"
        />
      </div>
      <Problem problem={problem} control="parent" />
      {failure !== null && <FieldError>{failure}</FieldError>}
    </Field>
  );
}
