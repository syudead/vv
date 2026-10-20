import { ArrowLeft, CircleAlert, Crop } from "lucide-react";
import { useLayoutEffect, useState } from "react";

import type { Video } from "../../api/client";
import type { ExternalId } from "../../api/credits";
import {
  type WorkInfoFetch,
  workInfoFetchCoverUrl,
  workInfoFetchPersonImageUrl,
  type WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { useLocalToday } from "../../credits/person/useLocalToday";
import { errorText, t } from "../../i18n";
import { cn } from "../../lib/cn";
import { isNarrowVideo } from "../../lib/format";
import { Alert, AlertAction, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { PersonFace } from "../../ui/PersonFace";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { drawComposition } from "../thumbnailUpload/composeImage";
import UploadPreview from "../thumbnailUpload/UploadPreview";
import { Description, withoutScheme } from "../WorkInfo";
import {
  ChangeSections,
  CompareSection,
  ImageChoice,
  KeptFold,
  PairRow,
  RowLabel,
  ValueRow,
  ValueToggle,
} from "./compareParts";
import type { CoverFraming } from "./coverFraming";
import {
  chipHint,
  creditKind,
  type DetailChip,
  detailRow,
  fillValues,
  videoWorkInfo,
} from "./detailRows";
import { pluginLabel, selectableTags } from "./fetchRules";
import { Chip } from "./TagChips";
import TagPicker from "./TagPicker";
import {
  choosable,
  freshIds,
  freshUrls,
  ROLE_FIELDS,
  type RoleField,
  roleRow,
  type WholeField,
  type WholeRow,
  wholeRows,
  type WorkSelection,
} from "./workSelection";

export interface Fetched {
  result: WorkInfoFetch;
  source: string;
  plugin: WorkInfoPlugin;
  fromSearch: boolean;
}

const text = t.player.workInfoFetch;

interface CompareProps {
  video: Video;
  result: WorkInfoFetch;
  selection: WorkSelection;
  locked: boolean;
  onChange: (selection: WorkSelection) => void;
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

function WholeValue({
  field,
  props,
  tone,
  struck = false,
}: {
  field: WholeField;
  props: CompareProps;
  tone: "now" | "fetched";
  struck?: boolean;
}) {
  const { video, result } = props;
  const value =
    field === "title"
      ? tone === "now"
        ? video.title
        : result.title
      : (() => {
          const row = detailRow(field, videoWorkInfo(video), result);
          return row.kind === "single"
            ? tone === "now"
              ? row.now
              : row.fetched
            : undefined;
        })();
  if (value === undefined || value === "") return null;
  const quiet = cn(tone === "now" && "text-muted-foreground", struck && "line-through");
  if (field === "description")
    // Key by value so Fetch again collapses a changed description.
    return (
      <div className={quiet}>
        <Description
          key={value}
          text={value}
          tone={tone === "now" ? "muted" : "default"}
        />
      </div>
    );
  return (
    <span
      title={field === "title" ? value : undefined}
      className={cn(
        "min-w-0 tabular-nums wrap-anywhere",
        field === "title" && "line-clamp-2",
        quiet,
      )}
    >
      {value}
    </span>
  );
}

function withField(
  selection: WorkSelection,
  field: WholeField,
  on: boolean,
): WorkSelection {
  const rest = selection.fields.filter((value) => value !== field);
  return { ...selection, fields: on ? [...rest, field] : rest };
}

function WholeLabel({
  row,
  props,
  selectable,
}: {
  row: WholeRow;
  props: CompareProps;
  selectable: boolean;
}) {
  return (
    <RowLabel
      label={text.fields[row.field]}
      checked={selectable ? props.selection.fields.includes(row.field) : undefined}
      locked={props.locked}
      onChange={(on) => props.onChange(withField(props.selection, row.field, on))}
    />
  );
}

function WholePair({
  row,
  props,
  selectable,
}: {
  row: WholeRow;
  props: CompareProps;
  selectable: boolean;
}) {
  const struck = selectable && props.selection.fields.includes(row.field);
  return (
    <PairRow
      label={<WholeLabel row={row} props={props} selectable={selectable} />}
      now={<WholeValue field={row.field} props={props} tone="now" struck={struck} />}
      fetched={<WholeValue field={row.field} props={props} tone="fetched" />}
      indented={selectable}
    />
  );
}

function wholeSection(
  props: CompareProps,
  rows: readonly WholeRow[],
): { all: boolean; onAll: (on: boolean) => void } {
  return {
    all: rows.every((row) => props.selection.fields.includes(row.field)),
    onAll: (on) =>
      props.onChange(
        rows.reduce((next, row) => withField(next, row.field, on), props.selection),
      ),
  };
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
        {text.adjustCover}
      </Button>
    </div>
  );
}

/** The thumbnail is one of the fetched covers or the video's own, then framed when a cover is chosen. */
function CoverPicker({ props, cover }: { props: CompareProps; cover: CoverFraming }) {
  const { video, result, selection } = props;
  const chosen = selection.cover === null ? undefined : result.covers[selection.cover];
  return (
    <div className="flex flex-col gap-2">
      <ImageChoice
        title={text.fields.cover}
        wide
        value={selection.cover === null ? "keep" : String(selection.cover)}
        locked={props.locked}
        onChange={(value) =>
          props.onChange({ ...selection, cover: value === "keep" ? null : Number(value) })
        }
        options={[
          {
            value: "keep",
            label: text.keepThumbnail,
            image: (
              <VideoThumbnail>
                {video.thumbnailUrl !== undefined && (
                  <VideoThumbnailImage src={video.thumbnailUrl} alt={text.nowThumbnail} />
                )}
              </VideoThumbnail>
            ),
          },
          ...result.covers.map((size, index) => ({
            value: String(index),
            label: text.coverOption(index + 1),
            image: (
              <VideoThumbnail>
                <VideoThumbnailImage
                  src={workInfoFetchCoverUrl(video.id, result.fetchId, index)}
                  alt={text.fetchedCover(size.width, size.height)}
                />
              </VideoThumbnail>
            ),
          })),
        ]}
      />
      {chosen !== undefined && cover.image !== null && (
        <FramedCover
          cover={{ ...cover, image: cover.image }}
          label={text.fetchedCover(chosen.width, chosen.height)}
          locked={props.locked}
        />
      )}
    </div>
  );
}

function TagsSection({ props }: { props: CompareProps }) {
  const { video, result, selection } = props;
  const selectable = selectableTags(video, result);
  if (selectable.length === 0) return null;
  return (
    <CompareSection
      title={text.fields.tags}
      count={selectable.length}
      all={selectable.every((name) => selection.tags.includes(name))}
      locked={props.locked}
      onAll={(on) => props.onChange({ ...selection, tags: on ? selectable : [] })}
    >
      <div className="py-2">
        <TagPicker
          video={video}
          result={result}
          pick={{
            selected: selection.tags,
            locked: props.locked,
            onToggle: (name, on) =>
              props.onChange({
                ...selection,
                tags: on
                  ? [...new Set([...selection.tags, name])]
                  : selection.tags.filter((item) => item !== name),
              }),
          }}
        />
      </div>
    </CompareSection>
  );
}

type CreditList = "people" | "organizations";

function creditList(field: RoleField): CreditList {
  return field.startsWith("people.") ? "people" : "organizations";
}

function withAt(selection: WorkSelection, list: CreditList, at: number, on: boolean) {
  const rest = selection[list].filter((value) => value !== at);
  return { ...selection, [list]: on ? [...rest, at] : rest };
}

/** A role's fetched entries: a toggle for each the owner may choose, and the rest named quietly. */
function RoleRow({ field, props }: { field: RoleField; props: CompareProps }) {
  const today = useLocalToday();
  const { video, result, selection } = props;
  const row = roleRow(field, video, result);
  const list = creditList(field);
  const kind = creditKind(field);
  const chips = row.fetched.filter(choosable);
  const onVideo = row.fetched.filter(
    (chip) => chip.mark === "onVideo" && !choosable(chip),
  ).length;
  const notLinked = row.fetched.filter((chip) => chip.mark === "notLinked");
  if (chips.length === 0 && notLinked.length === 0) return null;
  const fills = chips.filter((chip) => chip.profile !== undefined);
  return (
    <ValueRow label={<span>{text.fields[field]}</span>}>
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex min-w-0 flex-wrap items-center gap-1">
          {chips.map((chip) => (
            <CreditToggle
              key={chip.key}
              chip={chip}
              hint={chip.fills === true ? text.fillsHint : chipHint(chip, kind)}
              pressed={chip.at !== undefined && selection[list].includes(chip.at)}
              props={props}
              onPressedChange={(on) => {
                if (chip.at !== undefined)
                  props.onChange(withAt(selection, list, chip.at, on));
              }}
            />
          ))}
          {notLinked.map((chip) => (
            <Chip
              key={chip.key}
              item={{ name: chip.name, mark: "notLinked", hint: chipHint(chip, kind) }}
            />
          ))}
          {onVideo > 0 && (
            <span className="text-xs text-muted-foreground">
              {text.onVideoCount(onVideo)}
            </span>
          )}
        </div>
        {fills.map((chip) => (
          <p key={chip.key} className="text-xs break-words text-muted-foreground">
            {text.fillsEmpty(chip.name, fillValues(chip.profile ?? {}, today))}
          </p>
        ))}
      </div>
    </ValueRow>
  );
}

function CreditToggle({
  chip,
  hint,
  pressed,
  props,
  onPressedChange,
}: {
  chip: DetailChip;
  hint: string | undefined;
  pressed: boolean;
  props: CompareProps;
  onPressedChange: (on: boolean) => void;
}) {
  const label = chip.mark === "new" ? `${chip.name} · ${text.newMark}` : chip.name;
  return (
    <ValueToggle
      label={label}
      hint={hint}
      pressed={pressed}
      disabled={props.locked}
      onPressedChange={onPressedChange}
      leading={
        chip.faceIndex === undefined ? undefined : (
          <PersonFace
            shape="chip"
            name=""
            src={workInfoFetchPersonImageUrl(
              props.video.id,
              props.result.fetchId,
              chip.faceIndex,
            )}
          />
        )
      }
    />
  );
}

function choosableAts(props: CompareProps, list: CreditList): number[] {
  return ROLE_FIELDS.filter((field) => creditList(field) === list).flatMap((field) =>
    roleRow(field, props.video, props.result)
      .fetched.filter(choosable)
      .flatMap((chip) => (chip.at === undefined ? [] : [chip.at])),
  );
}

function CreditsSection({ props }: { props: CompareProps }) {
  const people = choosableAts(props, "people");
  const organizations = choosableAts(props, "organizations");
  const fields = ROLE_FIELDS.filter(
    (field) => roleRow(field, props.video, props.result).fetched.length > 0,
  );
  const shown = fields.some((field) =>
    roleRow(field, props.video, props.result).fetched.some(
      (chip) => choosable(chip) || chip.mark === "notLinked",
    ),
  );
  if (!shown) return null;
  const { selection } = props;
  return (
    <CompareSection
      title={text.credits}
      count={people.length + organizations.length}
      all={
        people.every((at) => selection.people.includes(at)) &&
        organizations.every((at) => selection.organizations.includes(at))
      }
      locked={props.locked}
      onAll={(on) =>
        props.onChange({
          ...selection,
          people: on ? people : [],
          organizations: on ? organizations : [],
        })
      }
    >
      {fields.map((field) => (
        <RoleRow key={field} field={field} props={props} />
      ))}
    </CompareSection>
  );
}

function idKey(id: ExternalId): string {
  return `${id.source}\u0000${id.value}`;
}

function ListsSection({ props }: { props: CompareProps }) {
  const { video, result, selection } = props;
  const urls = freshUrls(video, result);
  const ids = freshIds(video, result);
  if (urls.length + ids.length === 0) return null;
  const chosenIds = new Set(selection.externalIds.map(idKey));
  return (
    <CompareSection
      title={t.common.addsToLists}
      count={urls.length + ids.length}
      all={
        urls.every((url) => selection.urls.includes(url)) &&
        ids.every((id) => chosenIds.has(idKey(id)))
      }
      locked={props.locked}
      onAll={(on) =>
        props.onChange({ ...selection, urls: on ? urls : [], externalIds: on ? ids : [] })
      }
    >
      {urls.length > 0 && (
        <ValueRow label={<span>{text.fields.urls}</span>}>
          <div className="flex min-w-0 flex-wrap items-center gap-1">
            {urls.map((url) => (
              <ValueToggle
                key={url}
                label={withoutScheme(url)}
                hint={url}
                pressed={selection.urls.includes(url)}
                disabled={props.locked}
                onPressedChange={(on) =>
                  props.onChange({
                    ...selection,
                    urls: on
                      ? [...selection.urls, url]
                      : selection.urls.filter((item) => item !== url),
                  })
                }
              />
            ))}
          </div>
        </ValueRow>
      )}
      {ids.length > 0 && (
        <ValueRow label={<span>{text.fields.externalIds}</span>}>
          <div className="flex min-w-0 flex-wrap items-center gap-1">
            {ids.map((id) => (
              <ValueToggle
                key={idKey(id)}
                label={t.player.workInfo.externalId(id.source, id.value)}
                pressed={chosenIds.has(idKey(id))}
                disabled={props.locked}
                onPressedChange={(on) =>
                  props.onChange({
                    ...selection,
                    externalIds: on
                      ? [...selection.externalIds, id]
                      : selection.externalIds.filter((item) => idKey(item) !== idKey(id)),
                  })
                }
              />
            ))}
          </div>
        </ValueRow>
      )}
    </CompareSection>
  );
}

/**
 * A fetched work beside the video it would fill: the cover as a choice, the whole fields that fill
 * empty values or replace stored ones, the tags, the people and organizations and the lists one
 * by one, and the equal values folded.
 */
function Comparison({ props, cover }: { props: CompareProps; cover: CoverFraming }) {
  const rows = wholeRows(props.video, props.result);
  const fills = rows.filter((row) => row.kind === "fill");
  const replaces = rows.filter((row) => row.kind === "replace");
  const same = rows.filter((row) => row.kind === "same");
  return (
    <div className="flex flex-col gap-4 text-sm">
      {props.result.covers.length > 0 && <CoverPicker props={props} cover={cover} />}
      <ChangeSections
        fills={fills}
        replaces={replaces}
        locked={props.locked}
        keyOf={(row) => row.field}
        section={(part) => wholeSection(props, part)}
        fill={(row) => (
          <ValueRow label={<WholeLabel row={row} props={props} selectable />}>
            <WholeValue field={row.field} props={props} tone="fetched" />
          </ValueRow>
        )}
        replace={(row) => <WholePair row={row} props={props} selectable />}
      />
      <TagsSection props={props} />
      <CreditsSection props={props} />
      <ListsSection props={props} />
      {same.length > 0 && (
        <KeptFold count={same.length} note={text.sameNote}>
          {same.map((row) => (
            <WholePair key={row.field} row={row} props={props} selectable={false} />
          ))}
        </KeptFold>
      )}
    </div>
  );
}

export default function CompareStep({
  video,
  fetched,
  selection,
  locked,
  stale,
  refetching,
  failure,
  cover,
  onChange,
  onBack,
  onFetchAgain,
}: {
  video: Video;
  fetched: Fetched;
  selection: WorkSelection;
  locked: boolean;
  stale: boolean;
  refetching: boolean;
  failure: unknown;
  cover: CoverFraming;
  onChange: (selection: WorkSelection) => void;
  onBack: () => void;
  onFetchAgain: () => void;
}) {
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm break-words text-muted-foreground">
        {text.source(pluginLabel(fetched.plugin))}
        {text.sourceId(fetched.source)}
      </p>
      <Button
        variant="ghost"
        size="sm"
        className="self-start"
        disabled={locked || refetching}
        onClick={onBack}
      >
        <ArrowLeft aria-hidden="true" />
        {fetched.fromSearch ? text.back : text.backToFind}
      </Button>
      <CompareNotices
        stale={stale}
        refetching={refetching}
        failure={failure}
        onFetchAgain={onFetchAgain}
      />
      <Comparison
        props={{
          video,
          result: fetched.result,
          selection,
          locked: locked || stale,
          onChange,
        }}
        cover={cover}
      />
      {cover.adjusting && cover.image !== null && (
        <UploadPreview
          videoId={video.id}
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
