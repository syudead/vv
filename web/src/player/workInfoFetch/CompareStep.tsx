import { ArrowLeft, CircleAlert } from "lucide-react";
import { type ReactNode, useId } from "react";

import type { Video } from "../../api/client";
import type {
  WorkInfoFetch,
  WorkInfoField,
  WorkInfoPlugin,
  WorkInfoTarget,
} from "../../api/workInfoPlugins";
import { errorText, t } from "../../i18n";
import { Alert, AlertAction, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Checkbox } from "../../ui/shadcn/checkbox";
import { Label } from "../../ui/shadcn/label";
import { Spinner } from "../../ui/shadcn/spinner";
import { cn } from "../../lib/cn";
import { Description } from "../WorkInfo";
import {
  chipHint,
  creditKind,
  type DetailChip,
  detailRow,
  type ListRow,
  videoWorkInfo,
} from "./detailRows";
import {
  comparedFields,
  hostOf,
  pluginLabel,
  rowState,
  type RowState,
  videoTagNames,
} from "./fetchRules";
import TagChips, { type ChipItem, chipsOfVideo } from "./TagChips";

/** Fetched は比較の段に出す取得の結果と、それをどこから取ったかである。 */
export interface Fetched {
  result: WorkInfoFetch;
  /** 取った作品の URL（fetch）か ID（lookup）。「Fetch again」も同じもので走らせる。 */
  source: WorkInfoTarget;
  /** 取得に使ったプラグイン。「Fetch again」も同じプラグインで走らせる。 */
  plugin: WorkInfoPlugin;
  /** 検索の候補から取った。「Back to the matches」を出す。 */
  fromSearch: boolean;
}

/** Cell は比較の行の値の 1 つで、狭い画面では見出しを前に置く。 */
function Cell({ heading, children }: { heading: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 sm:col-span-2">
      <span className="text-xs text-muted-foreground sm:hidden">{heading}</span>
      {children}
    </div>
  );
}

function TitleValue({ title, tone }: { title: string; tone: "now" | "fetched" }) {
  return (
    <span
      title={title}
      className={
        tone === "now"
          ? "line-clamp-2 min-w-0 break-words text-muted-foreground"
          : "line-clamp-2 min-w-0 break-words text-foreground"
      }
    >
      {title}
    </span>
  );
}

/** StateNote は選べない行の理由（`Not in the result`・`Same as now`）である。 */
function StateNote({ state, noteId }: { state: RowState; noteId: string }) {
  const text = t.player.workInfoFetch;
  return (
    <span id={noteId} className="text-muted-foreground">
      {state === "missing" ? text.notInResult : text.sameAsNow}
    </span>
  );
}

function TagsValue({
  video,
  result,
  tone,
}: {
  video: Video;
  result: WorkInfoFetch;
  tone: "now" | "fetched";
}) {
  if (tone === "now") return <TagChips items={chipsOfVideo(video.tags)} />;
  const names = videoTagNames(video);
  const fresh = result.tags.filter((name) => !names.has(name)).length;
  return (
    <TagChips
      items={result.tags.map((name) => ({
        name,
        tentative: true,
        mark: names.has(name) ? "onVideo" : undefined,
      }))}
      trailing={t.player.workInfoFetch.newCount(fresh)}
    />
  );
}

function SingleValue({
  field,
  value,
  tone,
}: {
  field: WorkInfoField;
  value: string | undefined;
  tone: "now" | "fetched";
}) {
  if (value === undefined) return null;
  if (field === "description")
    // 鍵を値にし、「Fetch again」で説明が変われば畳んだ状態から出し直す。
    return (
      <Description key={value} text={value} tone={tone === "now" ? "muted" : "default"} />
    );
  return (
    <span
      className={cn(
        "min-w-0 tabular-nums wrap-anywhere",
        tone === "now" ? "text-muted-foreground" : "text-foreground",
      )}
    >
      {value}
    </span>
  );
}

function chipItems(
  chips: readonly DetailChip[],
  field: WorkInfoField,
  tone: "now" | "fetched",
): ChipItem[] {
  const kind = creditKind(field);
  return chips.map((chip) => ({
    key: chip.key,
    name: chip.name,
    full: chip.full,
    numeric: field === "externalIds",
    mark: tone === "now" ? undefined : chip.mark,
    hint: tone === "now" ? undefined : chipHint(chip, kind),
  }));
}

function ListValue({
  field,
  row,
  tone,
  noteId,
}: {
  field: WorkInfoField;
  row: ListRow;
  tone: "now" | "fetched";
  noteId: string;
}) {
  const text = t.player.workInfoFetch;
  if (tone === "now") return <TagChips items={chipItems(row.now, field, "now")} />;
  const nothing = row.state === "nothing";
  return (
    <TagChips
      items={chipItems(row.fetched, field, "fetched")}
      trailing={nothing ? text.nothingToAdd : text.newCount(row.fresh)}
      trailingId={nothing ? noteId : undefined}
    />
  );
}

/** Value は行の今か取得した側の値である。取得した側で選べない行は理由を出す。 */
function Value({
  field,
  video,
  result,
  tone,
  noteId,
}: {
  field: WorkInfoField;
  video: Video;
  result: WorkInfoFetch;
  tone: "now" | "fetched";
  noteId: string;
}) {
  const state = rowState(field, video, result);
  if (tone === "fetched" && (state === "missing" || state === "same"))
    return <StateNote state={state} noteId={noteId} />;
  if (field === "title")
    return (
      <TitleValue
        title={tone === "now" ? video.title : (result.title ?? "")}
        tone={tone}
      />
    );
  if (field === "tags") return <TagsValue video={video} result={result} tone={tone} />;
  const row = detailRow(field, videoWorkInfo(video), result);
  if (row.kind === "list")
    return <ListValue field={field} row={row} tone={tone} noteId={noteId} />;
  return (
    <SingleValue
      field={field}
      value={tone === "now" ? row.now : row.fetched}
      tone={tone}
    />
  );
}

function CompareRow({
  field,
  video,
  result,
  selected,
  locked,
  onToggle,
}: {
  field: WorkInfoField;
  video: Video;
  result: WorkInfoFetch;
  selected: boolean;
  locked: boolean;
  onToggle: (field: WorkInfoField, on: boolean) => void;
}) {
  const text = t.player.workInfoFetch;
  const id = useId();
  const state = rowState(field, video, result);
  const selectable = state === "differs";
  return (
    <div className="grid gap-x-4 gap-y-1 py-2 sm:grid-cols-5">
      <div className="flex items-center gap-2 self-start">
        <Checkbox
          id={`${id}-check`}
          checked={selectable && selected}
          disabled={!selectable || locked}
          aria-describedby={selectable ? undefined : `${id}-note`}
          onCheckedChange={(checked) => onToggle(field, checked === true)}
        />
        <Label htmlFor={`${id}-check`} className="font-normal text-muted-foreground">
          {text.fields[field]}
        </Label>
      </div>
      <Cell heading={text.now}>
        <Value
          field={field}
          video={video}
          result={result}
          tone="now"
          noteId={`${id}-note`}
        />
      </Cell>
      <Cell heading={text.fetched}>
        <Value
          field={field}
          video={video}
          result={result}
          tone="fetched"
          noteId={`${id}-note`}
        />
      </Cell>
    </div>
  );
}

/**
 * CompareStep は取得の結果を今の値と項目ごとに見比べる段である（ui-design「Compare」、
 * specs/052-plugin-work-info-fields/ui-design.md「Compare」）。題名とタグの後に、結果に値の
 * ある作品の詳細を 1 項目 1 行で並べる。取得した側を本文の色、今の側を従の色にし、値が
 * 無いか今と同じか足せる件の無い項目は選べない。期限切れなら
 * 「Fetch again」を添えた知らせを、他の失敗なら理由を、比較の上に出す。
 */
export default function CompareStep({
  video,
  fetched,
  selection,
  locked,
  stale,
  refetching,
  failure,
  onToggle,
  onBack,
  onFetchAgain,
}: {
  video: Video;
  fetched: Fetched;
  selection: readonly WorkInfoField[];
  /** 反映を送っている間。選択を変えられない。 */
  locked: boolean;
  stale: boolean;
  /** 「Fetch again」が走っている。 */
  refetching: boolean;
  failure: unknown;
  onToggle: (field: WorkInfoField, on: boolean) => void;
  onBack: () => void;
  onFetchAgain: () => void;
}) {
  const text = t.player.workInfoFetch;
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm break-words text-muted-foreground">
        {text.source(pluginLabel(fetched.plugin))}
        {"id" in fetched.source ? (
          text.sourceId(fetched.source.id)
        ) : (
          <a
            href={fetched.source.url}
            target="_blank"
            rel="noopener noreferrer"
            className="underline-offset-4 hover:text-primary hover:underline"
          >
            {hostOf(fetched.source.url)}
          </a>
        )}
      </p>
      {fetched.fromSearch && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start"
          disabled={locked || refetching}
          onClick={onBack}
        >
          <ArrowLeft aria-hidden="true" />
          {text.back}
        </Button>
      )}
      {stale && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{text.stale}</AlertTitle>
          <AlertAction>
            <Button
              variant="outline"
              size="sm"
              aria-disabled={refetching || undefined}
              aria-busy={refetching || undefined}
              onClick={() => {
                if (!refetching) onFetchAgain();
              }}
            >
              {refetching && <Spinner aria-hidden="true" role={undefined} />}
              {text.fetchAgain}
            </Button>
          </AlertAction>
        </Alert>
      )}
      {!stale && failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{text.applyFailed(errorText(failure))}</AlertTitle>
        </Alert>
      )}
      <div className="flex flex-col text-sm">
        <div className="hidden gap-x-4 pb-1 text-xs text-muted-foreground sm:grid sm:grid-cols-5">
          <span />
          <span className="col-span-2">{text.now}</span>
          <span className="col-span-2">{text.fetched}</span>
        </div>
        <div className="flex flex-col divide-y divide-border border-y border-border">
          {comparedFields(video, fetched.result).map((field) => (
            <CompareRow
              key={field}
              field={field}
              video={video}
              result={fetched.result}
              selected={selection.includes(field)}
              locked={locked || stale}
              onToggle={onToggle}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
