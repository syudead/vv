import { ArrowLeft, CircleAlert, Crop } from "lucide-react";
import { type ReactNode, useId, useLayoutEffect, useState } from "react";

import type { Video } from "../../api/client";
import {
  type WorkInfoFetch,
  workInfoFetchCoverUrl,
  type WorkInfoField,
  workInfoFetchPersonImageUrl,
  type WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { useLocalToday } from "../../credits/person/useLocalToday";
import { errorText, t } from "../../i18n";
import { isNarrowVideo } from "../../lib/format";
import { Alert, AlertAction, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Checkbox } from "../../ui/shadcn/checkbox";
import { Label } from "../../ui/shadcn/label";
import { Spinner } from "../../ui/shadcn/spinner";
import { cn } from "../../lib/cn";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { drawComposition } from "../thumbnailUpload/composeImage";
import UploadPreview from "../thumbnailUpload/UploadPreview";
import { Description } from "../WorkInfo";
import type { CoverFraming } from "./coverFraming";
import {
  chipHint,
  creditKind,
  type DetailChip,
  detailRow,
  fillValues,
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
import TagChips, { type ChipItem, chipsOfVideo, FoldButton } from "./TagChips";
import TagPicker, { type TagPick } from "./TagPicker";

export interface Fetched {
  result: WorkInfoFetch;
  source: string;
  plugin: WorkInfoPlugin;
  fromSearch: boolean;
}

export function Cell({ heading, children }: { heading: string; children: ReactNode }) {
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
  faceSrc?: (index: number) => string,
): ChipItem[] {
  const kind = creditKind(field);
  return chips.map((chip) => ({
    key: chip.key,
    name: chip.name,
    full: chip.full,
    numeric: field === "externalIds",
    mark: tone === "now" ? undefined : chip.mark,
    hint: tone === "now" ? undefined : chipHint(chip, kind),
    faceSrc:
      chip.faceIndex === undefined || faceSrc === undefined
        ? undefined
        : faceSrc(chip.faceIndex),
  }));
}

const FILL_FOLD = 3;

/** One line per fetched person with a profile, folded after three so the chips stay primary. */
function FillLines({ chips }: { chips: readonly DetailChip[] }) {
  const text = t.player.workInfoFetch;
  const today = useLocalToday();
  const [open, setOpen] = useState(false);
  const lines = chips.flatMap((chip) =>
    chip.profile === undefined
      ? []
      : [
          {
            key: chip.key,
            line: text.fillsEmpty(chip.name, fillValues(chip.profile, today)),
          },
        ],
  );
  if (lines.length === 0) return null;
  const folded = !open && lines.length > FILL_FOLD;
  const shown = folded ? lines.slice(0, FILL_FOLD) : lines;
  return (
    <div className="flex min-w-0 basis-full flex-col items-start text-xs text-muted-foreground">
      {shown.map(({ key, line }) => (
        <p key={key} title={line} className="max-w-full truncate">
          {line}
        </p>
      ))}
      {lines.length > FILL_FOLD && (
        <FoldButton
          hidden={lines.length - FILL_FOLD}
          folded={folded}
          onToggle={() => setOpen(folded)}
        />
      )}
    </div>
  );
}

function ListValue({
  field,
  row,
  tone,
  noteId,
  faceSrc,
}: {
  field: WorkInfoField;
  row: ListRow;
  tone: "now" | "fetched";
  noteId: string;
  faceSrc: (index: number) => string;
}) {
  const text = t.player.workInfoFetch;
  if (tone === "now") return <TagChips items={chipItems(row.now, field, "now")} />;
  const nothing = row.state === "nothing";
  // A row of people already on the video differs only by what it fills, which the fill lines say.
  const trailing = nothing
    ? text.nothingToAdd
    : row.fresh > 0
      ? text.newCount(row.fresh)
      : undefined;
  return (
    <>
      <TagChips
        items={chipItems(row.fetched, field, "fetched", faceSrc)}
        trailing={trailing}
        trailingId={nothing ? noteId : undefined}
      />
      <FillLines chips={row.fetched} />
    </>
  );
}

/** The cover drawn as the thumbnail it becomes, with the framing the owner chose. */
function FramedCover({
  cover,
  label,
  locked,
}: {
  cover: CoverFraming & { image: NonNullable<CoverFraming["image"]> };
  label: string;
  locked: boolean;
}) {
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null);
  const { image, aspect, fit } = cover;
  useLayoutEffect(() => {
    if (canvas !== null) drawComposition(canvas, image, aspect, fit);
  }, [canvas, image, aspect, fit]);
  return (
    <div className="flex w-full max-w-xs flex-col items-start gap-2">
      <VideoThumbnail>
        <canvas
          ref={setCanvas}
          role="img"
          aria-label={label}
          title={label}
          className="relative size-full object-contain"
        />
      </VideoThumbnail>
      <Button variant="outline" size="sm" disabled={locked} onClick={cover.adjust}>
        <Crop aria-hidden="true" />
        {t.player.workInfoFetch.adjustCover}
      </Button>
    </div>
  );
}

function CoverValue({
  video,
  result,
  tone,
  cover,
  locked,
}: {
  video: Video;
  result: WorkInfoFetch;
  tone: "now" | "fetched";
  cover: CoverFraming;
  locked: boolean;
}) {
  const text = t.player.workInfoFetch;
  const src =
    tone === "now" ? video.thumbnailUrl : workInfoFetchCoverUrl(video.id, result.fetchId);
  if (src === undefined) return null;
  const alt =
    tone === "fetched" && result.cover !== undefined
      ? text.fetchedCover(result.cover.width, result.cover.height)
      : text.nowThumbnail;
  if (tone === "fetched" && cover.image !== null)
    return (
      <FramedCover cover={{ ...cover, image: cover.image }} label={alt} locked={locked} />
    );
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
  cover,
}: {
  field: WorkInfoField;
  video: Video;
  result: WorkInfoFetch;
  tone: "now" | "fetched";
  noteId: string;
  tagPick: TagPick;
  cover: CoverFraming;
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
  if (field === "cover")
    return (
      <CoverValue
        video={video}
        result={result}
        tone={tone}
        cover={cover}
        locked={tagPick.locked}
      />
    );
  const row = detailRow(field, videoWorkInfo(video), result);
  if (row.kind === "list")
    return (
      <ListValue
        field={field}
        row={row}
        tone={tone}
        noteId={noteId}
        faceSrc={(index) => workInfoFetchPersonImageUrl(video.id, result.fetchId, index)}
      />
    );
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
  cover,
}: {
  field: WorkInfoField;
  video: Video;
  result: WorkInfoFetch;
  selected: boolean;
  locked: boolean;
  onToggle: (field: WorkInfoField, on: boolean) => void;
  tagPick: TagPick;
  cover: CoverFraming;
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
          cover={cover}
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
          cover={cover}
        />
      </Cell>
    </div>
  );
}

/** Now and Fetched over the five-column rows from sm; narrower rows head each cell instead. */
export function CompareHeadings() {
  const text = t.player.workInfoFetch;
  return (
    <div className="hidden gap-x-4 pb-1 text-xs text-muted-foreground sm:grid sm:grid-cols-5">
      <span />
      <span className="col-span-2">{text.now}</span>
      <span className="col-span-2">{text.fetched}</span>
    </div>
  );
}

/** The stale result's Fetch again, or the last apply's failure, above a comparison. */
export function CompareNotices({
  stale,
  refetching,
  failure,
  onFetchAgain,
}: {
  stale: boolean;
  refetching: boolean;
  failure: unknown;
  onFetchAgain: () => void;
}) {
  const text = t.player.workInfoFetch;
  return (
    <>
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
    </>
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
  cover,
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
  cover: CoverFraming;
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
      <CompareNotices
        stale={stale}
        refetching={refetching}
        failure={failure}
        onFetchAgain={onFetchAgain}
      />
      <div className="flex flex-col text-sm">
        <CompareHeadings />
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
              cover={cover}
            />
          ))}
        </div>
      </div>
      {cover.adjusting && cover.image !== null && (
        <UploadPreview
          image={cover.image}
          name={pluginLabel(fetched.plugin)}
          aspect={cover.aspect}
          narrow={isNarrowVideo(video)}
          applying={false}
          initialFit={cover.fit}
          applyLabel={text.keepFraming}
          onApply={cover.keep}
          onCancel={cover.close}
        />
      )}
    </div>
  );
}
