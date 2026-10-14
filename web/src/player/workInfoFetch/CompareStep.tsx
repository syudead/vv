import { ArrowLeft, CircleAlert } from "lucide-react";
import { type ReactNode, useId } from "react";

import type { Video } from "../../api/client";
import type {
  WorkInfoFetch,
  WorkInfoField,
  WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { errorText, t } from "../../i18n";
import { Alert, AlertAction, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Checkbox } from "../../ui/shadcn/checkbox";
import { Label } from "../../ui/shadcn/label";
import { Spinner } from "../../ui/shadcn/spinner";
import {
  FIELDS,
  hostOf,
  pluginLabel,
  rowState,
  type RowState,
  videoTagNames,
} from "./fetchRules";
import TagChips, { chipsOfVideo } from "./TagChips";

/** Fetched は比較の段に出す取得の結果と、それをどこから取ったかである。 */
export interface Fetched {
  result: WorkInfoFetch;
  url: string;
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

function NowValue({ field, video }: { field: WorkInfoField; video: Video }) {
  if (field === "title") return <TitleValue title={video.title} tone="now" />;
  return <TagChips items={chipsOfVideo(video.tags)} />;
}

function FetchedValue({
  field,
  state,
  video,
  result,
  noteId,
}: {
  field: WorkInfoField;
  state: RowState;
  video: Video;
  result: WorkInfoFetch;
  noteId: string;
}) {
  const text = t.player.workInfoFetch;
  if (state !== "differs") {
    return (
      <span id={noteId} className="text-muted-foreground">
        {state === "missing" ? text.notInResult : text.sameAsNow}
      </span>
    );
  }
  if (field === "title") return <TitleValue title={result.title ?? ""} tone="fetched" />;
  const names = videoTagNames(video);
  const fresh = result.tags.filter((name) => !names.has(name)).length;
  return (
    <TagChips
      items={result.tags.map((name) => ({
        name,
        tentative: true,
        onVideo: names.has(name),
      }))}
      trailing={text.newCount(fresh)}
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
        <NowValue field={field} video={video} />
      </Cell>
      <Cell heading={text.fetched}>
        <FetchedValue
          field={field}
          state={state}
          video={video}
          result={result}
          noteId={`${id}-note`}
        />
      </Cell>
    </div>
  );
}

/**
 * CompareStep は取得の結果を今の値と項目ごとに見比べる段である（ui-design「Compare」）。
 * 取得した側を本文の色、今の側を従の色にし、値が無いか今と同じ項目は選べない。期限切れなら
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
        <a
          href={fetched.url}
          target="_blank"
          rel="noopener noreferrer"
          className="underline-offset-4 hover:text-primary hover:underline"
        >
          {hostOf(fetched.url)}
        </a>
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
          {FIELDS.map((field) => (
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
