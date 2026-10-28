import { CircleAlert, Crop } from "lucide-react";
import type { FormEvent } from "react";

import { t } from "../../i18n";
import { workInfoFetchCoverUrl } from "../../api/workInfoPlugins";
import UploadPreview from "../../player/thumbnailUpload/UploadPreview";
import FramedCoverCanvas from "../../player/workInfoFetch/FramedCoverCanvas";
import ThumbnailBackdrop from "../../ui/ThumbnailBackdrop";
import { useToast } from "../../ui/Toast";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { Alert, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/shadcn/select";
import { Spinner } from "../../ui/shadcn/spinner";
import { workSelectionEmpty } from "../../player/workInfoFetch/workSelection";
import { CreateDialogFrame } from "../NameDialogParts";
import {
  applicable,
  chosenWork,
  type FillRow,
  type FillTargets,
  useFillReview,
} from "./useFillReview";

const skip = "skip";

function Pending({ text }: { text: string }) {
  return (
    <span className="flex items-center gap-2 text-sm text-muted-foreground">
      <Spinner aria-hidden="true" />
      {text}
    </span>
  );
}

function RowChoice({
  row,
  disabled,
  onChoose,
}: {
  row: FillRow;
  disabled: boolean;
  onChoose: (choice: number | null) => void;
}) {
  const words = t.library.selection.fillReview;
  if (row.status === "waiting") {
    return <span className="text-sm text-muted-foreground">{words.waiting}</span>;
  }
  if (row.status === "searching") return <Pending text={words.searching} />;
  if (row.candidates.length === 0) {
    const text = row.status === "failed" ? row.failure : words.noResults;
    return <span className="text-sm text-muted-foreground">{text}</span>;
  }
  return (
    <Select
      value={row.choice === null ? skip : String(row.choice)}
      onValueChange={(value) => onChoose(value === skip ? null : Number(value))}
      disabled={disabled || row.status !== "searched"}
    >
      <SelectTrigger size="sm" aria-label={words.choiceFor(row.target.title)}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={skip}>{words.skip}</SelectItem>
        {row.candidates.map((candidate, i) => (
          <SelectItem key={`${String(i)}:${candidate.id}`} value={String(i)}>
            {candidate.title === "" ? candidate.id : candidate.title}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** The chosen work's code and release date, or where the row stands while its work is read or written. */
function RowPlan({ row }: { row: FillRow }) {
  const words = t.library.selection.fillReview;
  if (row.status === "fetching") return <Pending text={words.reading} />;
  if (row.status === "applying") return <Pending text={words.applying} />;
  if (row.status === "applied") return <p className="text-sm">{words.applied}</p>;
  if (row.status === "failed" && row.candidates.length > 0) {
    return <p className="text-sm text-destructive">{row.failure}</p>;
  }
  const work = chosenWork(row);
  if (work === undefined || row.plan?.workId !== work.id) return null;
  const { facts, selection } = row.plan.plan;
  const empty = workSelectionEmpty(selection);
  if (facts.length === 0 && !empty) return null;
  return (
    <p className="text-sm break-words text-muted-foreground">
      {[...facts, ...(empty ? [words.nothingToFill] : [])].join(" · ")}
    </p>
  );
}

/** The cover the row fills, drawn with its framing once the owner adjusted it, and the way to adjust it. */
function RowCover({
  row,
  disabled,
  opening,
  onAdjust,
}: {
  row: FillRow;
  disabled: boolean;
  opening: boolean;
  onAdjust: () => void;
}) {
  const work = chosenWork(row);
  const cover = row.plan?.cover;
  if (row.status !== "searched" || work === undefined || row.plan?.workId !== work.id)
    return null;
  if (cover === null || cover === undefined) return null;
  const src = workInfoFetchCoverUrl(row.target.videoId, row.plan.fetchId, cover.index);
  return (
    <div className="flex items-center gap-3">
      <VideoThumbnail className="w-list-thumb shrink-0 rounded-sm">
        {row.framing === null ? (
          <>
            <ThumbnailBackdrop src={src} />
            <VideoThumbnailImage src={src} alt={cover.label} title={cover.label} />
          </>
        ) : (
          <FramedCoverCanvas cover={row.framing} label={cover.label} />
        )}
      </VideoThumbnail>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled}
        aria-busy={opening || undefined}
        onClick={onAdjust}
      >
        {opening ? <Spinner aria-hidden="true" /> : <Crop aria-hidden="true" />}
        {t.player.workInfoFetch.adjustCover}
      </Button>
    </div>
  );
}

/**
 * Show what filling each target from its chosen work writes, let the owner change or drop each choice, and apply
 * the fetched results the review showed, one video at a time.
 */
export default function FillReviewDialog({
  pluginId,
  targets,
  onClose,
}: {
  pluginId: number;
  targets: FillTargets;
  onClose: () => void;
}) {
  const words = t.library.selection.fillReview;
  const toast = useToast();
  const review = useFillReview(pluginId, targets);
  const count = (review.rows ?? []).filter(applicable).length;
  const locked = review.applying || review.adjusting !== null;
  const adjustIndex = review.adjusting?.ready === true ? review.adjusting.index : null;
  const adjusted = adjustIndex === null ? undefined : review.rows?.[adjustIndex];
  const framing = adjusted?.framing ?? null;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (review.applying || count === 0) return;
    void review.apply().then((applied) => {
      toast(t.library.selection.filled(applied));
      if (applied === count) onClose();
    });
  }

  return (
    <CreateDialogFrame
      words={{ ...words, submit: words.submit(count) }}
      description={words.description}
      pending={review.applying}
      failure={null}
      submitDisabled={count === 0}
      size="wide"
      onSubmit={submit}
      onClose={onClose}
    >
      {review.failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{words.listFailed(review.failure)}</AlertTitle>
        </Alert>
      )}
      {review.rows === null && review.failure === null && (
        <Pending text={words.listing} />
      )}
      {review.rows?.length === 0 && (
        <p className="text-sm text-muted-foreground">{words.noTargets}</p>
      )}
      {review.rows !== null && review.rows.length > 0 && (
        <ul className="flex flex-col divide-y" aria-label={words.list}>
          {review.rows.map((row, index) => (
            <li key={row.target.videoId} className="flex flex-col gap-2 py-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="min-w-0">
                  <p className="text-sm font-medium break-words">{row.target.title}</p>
                  {row.target.query !== row.target.title && (
                    <p className="text-xs text-muted-foreground break-words">
                      {row.target.query}
                    </p>
                  )}
                </div>
                <RowChoice
                  row={row}
                  disabled={locked}
                  onChoose={(choice) => review.choose(index, choice)}
                />
              </div>
              <RowPlan row={row} />
              <RowCover
                row={row}
                disabled={locked}
                opening={review.adjusting?.index === index && !review.adjusting.ready}
                onAdjust={() => review.adjust(index)}
              />
            </li>
          ))}
        </ul>
      )}
      {adjustIndex !== null && adjusted !== undefined && framing !== null && (
        <UploadPreview
          videoId={adjusted.target.videoId}
          image={framing.image}
          name={adjusted.target.title}
          aspect={framing.aspect}
          narrow={adjusted.plan?.cover?.narrow ?? false}
          applying={false}
          initialFit={framing.fit}
          applyLabel={t.player.workInfoFetch.keepFraming}
          onApply={(fit, replaced) => review.keepFraming(adjustIndex, fit, replaced)}
          onCancel={review.closeFraming}
        />
      )}
    </CreateDialogFrame>
  );
}
