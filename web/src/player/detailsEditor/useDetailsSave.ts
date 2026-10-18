import { type ReactNode, type RefObject, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed, type Video } from "../../api/client";
import { createCredit } from "../../api/credits";
import { setVideoDetails } from "../../api/workInfo";
import { errorText, t } from "../../i18n";
import { type Draft, newEntries, toDetailsInput, withCreated } from "./draft";
import type { FieldProblem } from "./fields";

/** field holds a server path that could not be mapped to an input. */
export interface SaveFailure {
  text: ReactNode;
  field?: string;
}

/** Create new credits sequentially and retain their IDs before saving details once. Map field errors to inputs, report partial creation on failure, and abort when leaving. */
export function useDetailsSave({
  videoId,
  draftRef,
  setDraft,
  onSaved,
  onGone,
}: {
  videoId: number;
  draftRef: RefObject<Draft>;
  setDraft: (draft: Draft) => void;
  onSaved: (video: Video) => void;
  onGone: () => void;
}): {
  saving: boolean;
  failure: SaveFailure | null;
  problem: FieldProblem | null;
  save: () => void;
} {
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<SaveFailure | null>(null);
  const [problem, setProblem] = useState<FieldProblem | null>(null);
  const busy = useRef(false);
  const controller = useRef<AbortController | null>(null);

  useEffect(() => () => controller.current?.abort(), []);

  async function run(signal: AbortSignal): Promise<void> {
    const e = t.player.detailsEditor;
    let draft = draftRef.current;
    let created = 0;
    for (const entry of newEntries(draft)) {
      try {
        const credit = await createCredit(entry.kind, { name: entry.name }, signal);
        draft = withCreated(draft, entry.kind, entry.newKey, credit.id);
        created += 1;
        setDraft(draft);
      } catch (error) {
        if (signal.aborted || isAborted(error)) return;
        setFailure({ text: e.saveFailed(errorText(error)) });
        return;
      }
    }
    const { body, targets } = toDetailsInput(draft);
    let video: Video;
    try {
      video = await setVideoDetails(videoId, body, signal);
    } catch (error) {
      if (signal.aborted || isAborted(error)) return;
      if (error instanceof RequestFailed && error.reason === "video_not_found") {
        onGone();
        return;
      }
      const reason = errorText(error);
      const field =
        error instanceof RequestFailed && (error.status === 400 || error.status === 404)
          ? error.field
          : undefined;
      if (field !== undefined) {
        const control = targets.get(field);
        if (control !== undefined) setProblem({ control, text: reason });
        else setFailure({ text: e.saveFailed(reason), field });
        return;
      }
      setFailure({
        text: created > 0 ? e.createdNotSaved(created, reason) : e.saveFailed(reason),
      });
      return;
    }
    onSaved(video);
  }

  const save = () => {
    if (busy.current) return;
    busy.current = true;
    const mine = new AbortController();
    controller.current = mine;
    setSaving(true);
    setFailure(null);
    setProblem(null);
    void run(mine.signal).finally(() => {
      busy.current = false;
      if (!mine.signal.aborted) setSaving(false);
    });
  };

  return { saving, failure, problem, save };
}
