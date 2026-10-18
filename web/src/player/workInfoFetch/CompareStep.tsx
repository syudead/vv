import { ArrowLeft, CircleAlert } from "lucide-react";
import { type ReactNode, useId } from "react";

import type { Video } from "../../api/client";
import {
  type WorkInfoFetch,
  workInfoFetchCoverUrl,
  type WorkInfoField,
  type WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { errorText, t } from "../../i18n";
import { Alert, AlertAction, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Checkbox } from "../../ui/shadcn/checkbox";
import { Label } from "../../ui/shadcn/label";
import { Spinner } from "../../ui/shadcn/spinner";
import { cn } from "../../lib/cn";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
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
  pluginLabel,
  rowState,
  type RowState,
  selectableTags,
  tagsRowChecked,
} from "./fetchRules";
import TagChips, { type ChipItem, chipsOfVideo } from "./TagChips";
import TagPicker, { type TagPick } from "./TagPicker";

export interface Fetched {
  result: WorkInfoFetch;
  source: string;
  plugin: WorkInfoPlugin;
  fromSearch: boolean;
}

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

function StateNote({ state, noteId }: { state: RowState; noteId: string }) {
  const text = t.player.workInfoFetch;
  return (
    <span id={noteId} className="text-muted-foreground">
      {state === "missing" ? text.notInResult : text.sameAsNow}
    </span>
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
    // Key by value so Fetch again collapses a changed description.
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

function CoverValue({
  video,
  result,
  tone,
}: {
  video: Video;
  result: WorkInfoFetch;
  tone: "now" | "fetched";
}) {
  const text = t.player.workInfoFetch;
  const src =
    tone === "now" ? video.thumbnailUrl : workInfoFetchCoverUrl(video.id, result.fetchId);
  if (src === undefined) return null;
  const alt =
    tone === "fetched" && result.cover !== undefined
      ? text.fetchedCover(result.cover.width, result.cover.height)
      : text.nowThumbnail;
  return (
    <VideoThumbnail className={cn("max-w-xs", tone === "now" && "opacity-70")}>
      <VideoThumbnailImage src={src} alt={alt} title={alt} />
    </VideoThumbnail>
  );
}

function Value({
  field,
  video,
  result,
  tone,
  noteId,
  tagPick,
}: {
  field: WorkInfoField;
  video: Video;
  result: WorkInfoFetch;
  tone: "now" | "fetched";
  noteId: string;
  tagPick: TagPick;
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
  if (field === "tags")
    return tone === "now" ? (
      <TagChips items={chipsOfVideo(video.tags)} />
    ) : (
      <TagPicker video={video} result={result} pick={tagPick} />
    );
  if (field === "cover") return <CoverValue video={video} result={result} tone={tone} />;
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
  tagPick,
}: {
  field: WorkInfoField;
  video: Video;
  result: WorkInfoFetch;
  selected: boolean;
  locked: boolean;
  onToggle: (field: WorkInfoField, on: boolean) => void;
  tagPick: TagPick;
}) {
  const text = t.player.workInfoFetch;
  const id = useId();
  const state = rowState(field, video, result);
  const selectable = state === "differs";
  const checked =
    field === "tags"
      ? tagsRowChecked(selectableTags(video, result), tagPick.selected)
      : selected;
  return (
    <div className="grid gap-x-4 gap-y-1 py-2 sm:grid-cols-5">
      <div className="flex items-center gap-2 self-start">
        <Checkbox
          id={`${id}-check`}
          checked={selectable && checked}
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
          tagPick={tagPick}
        />
      </Cell>
      <Cell heading={text.fetched}>
        <Value
          field={field}
          video={video}
          result={result}
          tone="fetched"
          noteId={`${id}-note`}
          tagPick={tagPick}
        />
      </Cell>
    </div>
  );
}

export default function CompareStep({
  video,
  fetched,
  selection,
  tags,
  locked,
  stale,
  refetching,
  failure,
  onToggle,
  onToggleTag,
  onBack,
  onFetchAgain,
}: {
  video: Video;
  fetched: Fetched;
  selection: readonly WorkInfoField[];
  tags: readonly string[];
  locked: boolean;
  stale: boolean;
  refetching: boolean;
  failure: unknown;
  onToggle: (field: WorkInfoField, on: boolean) => void;
  onToggleTag: (name: string, on: boolean) => void;
  onBack: () => void;
  onFetchAgain: () => void;
}) {
  const text = t.player.workInfoFetch;
  const tagPick: TagPick = {
    selected: tags,
    locked: locked || stale,
    onToggle: onToggleTag,
  };
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm break-words text-muted-foreground">
        {text.source(pluginLabel(fetched.plugin))}
        {text.sourceId(fetched.source)}
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
              tagPick={tagPick}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
