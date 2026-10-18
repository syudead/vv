import { AlertCircle, LoaderCircle } from "lucide-react";
import {
  type ReactNode,
  type Ref,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { FormPage } from "../../ui/patterns/form-page";
import { Button } from "../../ui/shadcn/button";
import BasicsPanel from "./BasicsPanel";
import CreditsPanel from "./CreditsPanel";
import { type Draft, draftChanged, draftFromVideo } from "./draft";
import EditorAside from "./EditorAside";
import { controlId } from "./fields";
import { type SaveFailure, useDetailsSave } from "./useDetailsSave";

export function EditorBand({
  saving,
  saveDisabled,
  onCancel,
  onSave,
  cancelRef,
}: {
  saving: boolean;
  saveDisabled: boolean;
  onCancel: () => void;
  onSave: () => void;
  cancelRef?: Ref<HTMLButtonElement>;
}) {
  const e = t.player.detailsEditor;
  return (
    <>
      <Button
        ref={cancelRef}
        variant="ghost"
        size="sm"
        aria-disabled={saving || undefined}
        onClick={() => {
          if (!saving) onCancel();
        }}
      >
        {e.cancel}
      </Button>
      <Button
        size="sm"
        aria-disabled={saving || saveDisabled || undefined}
        onClick={() => {
          if (!saving && !saveDisabled) onSave();
        }}
      >
        {saving && (
          <LoaderCircle
            className="animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        )}
        {saving ? e.saving : e.save}
      </Button>
    </>
  );
}

export function FailureLine({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="flex items-center gap-2 text-sm text-destructive">
      <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 wrap-anywhere">{children}</span>
    </p>
  );
}

function failureText(failure: SaveFailure): ReactNode {
  if (failure.field === undefined) return failure.text;
  return (
    <>
      {failure.text} <code className="font-mono text-xs">{failure.field}</code>
    </>
  );
}

export default function DetailsForm({
  video,
  backTo,
  onLeave,
  onSaved,
  onGone,
  onChangedChange,
  onSavingChange,
  cancelRef,
  backRef,
}: {
  video: Video;
  backTo: string;
  onLeave: () => void;
  onSaved: (video: Video) => void;
  onGone: () => void;
  onChangedChange: (changed: boolean) => void;
  onSavingChange: (saving: boolean) => void;
  cancelRef?: Ref<HTMLButtonElement>;
  backRef?: Ref<HTMLAnchorElement>;
}) {
  const [loaded] = useState(() => draftFromVideo(video));
  const [draft, setDraft] = useState<Draft>(loaded);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const nameRef = useRef<HTMLInputElement | null>(null);
  const { saving, failure, problem, save } = useDetailsSave({
    videoId: video.id,
    draftRef,
    setDraft,
    onSaved,
    onGone,
  });

  const changed = draftChanged(draft, loaded);
  useEffect(() => onChangedChange(changed), [changed, onChangedChange]);
  useEffect(() => onSavingChange(saving), [saving, onSavingChange]);

  useEffect(() => {
    nameRef.current?.focus();
  }, []);

  useLayoutEffect(() => {
    if (problem === null) return;
    const element = document.getElementById(controlId(problem.control));
    // jsdom does not implement scrollIntoView.
    element?.scrollIntoView?.({ block: "nearest" });
    element?.focus();
  }, [problem]);

  const panelProps = {
    draft,
    update: (change: (current: Draft) => Draft) => setDraft((current) => change(current)),
    saving,
    problem,
    save,
  };

  return (
    <FormPage
      title={t.player.detailsEditor.title}
      detail={video.title}
      actions={
        <EditorBand
          saving={saving}
          saveDisabled={false}
          onCancel={onLeave}
          onSave={save}
          cancelRef={cancelRef}
        />
      }
      notice={
        failure === null ? undefined : <FailureLine>{failureText(failure)}</FailureLine>
      }
      asideLabel={t.player.detailsEditor.video}
      aside={
        <EditorAside video={video} backTo={backTo} onBack={onLeave} backRef={backRef} />
      }
      panels={[
        {
          key: "basics",
          legend: t.player.detailsEditor.basics,
          content: (
            <BasicsPanel
              {...panelProps}
              namePlaceholder={video.fileTitle ?? video.title}
              nameRef={nameRef}
            />
          ),
        },
        {
          key: "credits",
          legend: t.player.detailsEditor.credits,
          content: <CreditsPanel {...panelProps} />,
        },
      ]}
    />
  );
}
