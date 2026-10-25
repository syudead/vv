import { CircleAlert } from "lucide-react";
import { type FormEvent, useState } from "react";

import { startWorkInfoFills } from "../../api/workInfoPlugins";
import { errorText, t, type UiText } from "../../i18n";
import { useToast } from "../../ui/Toast";
import { Alert, AlertTitle } from "../../ui/shadcn/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/shadcn/select";
import { Spinner } from "../../ui/shadcn/spinner";
import { CreateDialogFrame } from "../NameDialogParts";
import {
  chosenItems,
  type FillRow,
  type FillTargets,
  useFillReview,
} from "./useFillReview";

const skip = "skip";

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
  if (row.status === "waiting" || row.status === "searching") {
    return (
      <span className="flex items-center gap-2 text-sm text-muted-foreground">
        {row.status === "searching" && <Spinner aria-hidden="true" />}
        {row.status === "searching" ? words.searching : words.waiting}
      </span>
    );
  }
  if (row.status === "failed") {
    return <span className="text-sm text-destructive">{row.failure}</span>;
  }
  if (row.candidates.length === 0) {
    return <span className="text-sm text-muted-foreground">{words.noResults}</span>;
  }
  return (
    <Select
      value={row.choice === null ? skip : String(row.choice)}
      onValueChange={(value) => onChoose(value === skip ? null : Number(value))}
      disabled={disabled}
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

/**
 * Show each target's search result with its match preselected, let the owner change or drop each choice, and
 * queue fills of the chosen works only.
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
  const items = chosenItems(review.rows ?? []);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || items.length === 0) return;
    setPending(true);
    setFailure(null);
    startWorkInfoFills(pluginId, items).then(
      (queued) => {
        toast(t.library.selection.fillsQueued(queued.queued));
        onClose();
      },
      (error: unknown) => {
        setPending(false);
        setFailure(words.failed(errorText(error)));
      },
    );
  }

  return (
    <CreateDialogFrame
      words={{ ...words, submit: words.submit(items.length) }}
      description={words.description}
      pending={pending}
      failure={failure}
      submitDisabled={items.length === 0}
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
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner aria-hidden="true" />
          {words.listing}
        </p>
      )}
      {review.rows?.length === 0 && (
        <p className="text-sm text-muted-foreground">{words.noTargets}</p>
      )}
      {review.rows !== null && review.rows.length > 0 && (
        <ul className="flex flex-col divide-y" aria-label={words.list}>
          {review.rows.map((row, index) => (
            <li
              key={row.target.videoId}
              className="flex flex-wrap items-center justify-between gap-2 py-2"
            >
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
                disabled={pending}
                onChoose={(choice) => review.choose(index, choice)}
              />
            </li>
          ))}
        </ul>
      )}
    </CreateDialogFrame>
  );
}
