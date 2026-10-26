import { type ReactNode, type RefObject, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed, type Video } from "../../api/client";
import { createCredit, updateCredit } from "../../api/credits";
import { setVideoDetails } from "../../api/workInfo";
import { errorText, t } from "../../i18n";
import { isRemoteId } from "../../lib/remoteId";
import { Button } from "../../ui/shadcn/button";
import {
  type Draft,
  namings,
  newEntries,
  toDetailsInput,
  withCreated,
  withNamed,
} from "./draft";
import type { FieldProblem } from "./fields";

/** field holds a server path that could not be mapped to an input. */
export interface SaveFailure {
  text: ReactNode;
  field?: string;
}

export interface EditorSave {
  saving: boolean;
  failure: SaveFailure | null;
  problem: FieldProblem | null;
  save: () => void;
}

/**
 * One save at a time for a full-page editor: each run clears the previous failure and gets a
 * signal that aborts when the editor unmounts.
 */
export function useEditorSave(run: (signal: AbortSignal) => Promise<void>): EditorSave & {
  setFailure: (failure: SaveFailure) => void;
  setProblem: (problem: FieldProblem) => void;
} {
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<SaveFailure | null>(null);
  const [problem, setProblem] = useState<FieldProblem | null>(null);
  const busy = useRef(false);
  const controller = useRef<AbortController | null>(null);

  useEffect(() => () => controller.current?.abort(), []);

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

  return { saving, failure, problem, save, setFailure, setProblem };
}

/** Name chosen provisional people, then create new credits sequentially and retain their IDs before saving details once; the peer's video sends new credits by name instead. Map field errors to inputs, report partial creation on failure, and abort when leaving. */
export function useDetailsSave({
  video,
  draftRef,
  setDraft,
  onSaved,
  onGone,
  onReload,
}: {
  video: Pick<Video, "id" | "virtual" | "contentVersion">;
  draftRef: RefObject<Draft>;
  setDraft: (draft: Draft) => void;
  onSaved: (video: Video) => void;
  onGone: () => void;
  /** Re-read a video that changed in another tab, keeping the editor open. */
  onReload: () => void;
}): EditorSave {
  const { saving, failure, problem, save, setFailure, setProblem } = useEditorSave(run);

  async function run(signal: AbortSignal): Promise<void> {
    const e = t.player.detailsEditor;
    let draft = draftRef.current;
    // A virtual video's title is its only name; refuse an empty one before anything is written.
    if (video.virtual && draft.name.trim() === "") {
      setProblem({ control: "name", text: t.errors.reason.invalid_title });
      return;
    }
    let created = 0;
    // Names go before every other write, so a refused name stops the save before the video changes.
    for (const naming of namings(draft)) {
      try {
        await updateCredit("person", naming.id, { name: naming.name }, signal);
        draft = withNamed(draft, naming.id, naming.name);
        setDraft(draft);
      } catch (error) {
        if (signal.aborted || isAborted(error)) return;
        setFailure({ text: e.saveFailed(errorText(error)) });
        return;
      }
    }
    // The peer's video sends new credits by name, so they are created on the peer and not here.
    const byName = isRemoteId(video.id);
    for (const entry of byName ? [] : newEntries(draft)) {
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
    const { body, targets } = toDetailsInput(draft, { byName });
    let saved: Video;
    try {
      saved = await setVideoDetails(
        video.id,
        { ...body, contentVersion: video.contentVersion },
        signal,
      );
    } catch (error) {
      if (signal.aborted || isAborted(error)) return;
      if (error instanceof RequestFailed && error.reason === "video_not_found") {
        onGone();
        return;
      }
      if (error instanceof RequestFailed && error.reason === "content_changed") {
        setFailure({ text: <ChangedNotice onReload={onReload} /> });
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
    onSaved(saved);
  }

  return { saving, failure, problem, save };
}

function ChangedNotice({ onReload }: { onReload: () => void }) {
  return (
    <>
      {t.player.virtual.changed}{" "}
      <Button
        variant="link"
        size="sm"
        className="h-auto px-0 py-0 align-baseline"
        onClick={onReload}
      >
        {t.common.reload}
      </Button>
    </>
  );
}
