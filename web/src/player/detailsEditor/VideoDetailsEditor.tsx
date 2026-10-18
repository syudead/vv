import { useCallback, useEffect, useState } from "react";
import { useParams } from "react-router";

import { getVideo, isAborted, RequestFailed, type Video } from "../../api/client";
import { errorText, t, type UiText } from "../../i18n";
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { ErrorState } from "../../ui/patterns/error-state";
import { FormPage } from "../../ui/patterns/form-page";
import { Field, FieldTitle } from "../../ui/shadcn/field";
import { Skeleton } from "../../ui/shadcn/skeleton";
import DetailsForm from "./DetailsForm";
import { EditorBand } from "./EditorBand";
import EditorAside from "./EditorAside";
import { clearVideoForEditor, handBackToPage, peekVideoForEditor } from "./handoff";
import { useLeaving } from "./useLeaving";

type Load =
  | { kind: "loading" }
  | { kind: "ready"; video: Video }
  | { kind: "failed"; reason: UiText }
  | { kind: "gone" };

function parseId(raw: string | undefined): number {
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 0;
}

/** Start from the page's handoff when available; direct URLs fetch the video. */
function useVideoLoad(id: number): {
  load: Load;
  setLoad: (load: Load) => void;
  retry: () => void;
} {
  const [load, setLoad] = useState<Load>(() => {
    const video = peekVideoForEditor(id);
    if (video !== undefined) return { kind: "ready", video };
    return id === 0 ? { kind: "gone" } : { kind: "loading" };
  });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => clearVideoForEditor(), []);

  const loading = load.kind === "loading";
  useEffect(() => {
    if (!loading) return;
    const controller = new AbortController();
    getVideo(id, controller.signal).then(
      (video) => {
        if (!controller.signal.aborted) setLoad({ kind: "ready", video });
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        if (error instanceof RequestFailed && error.status === 404)
          setLoad({ kind: "gone" });
        else setLoad({ kind: "failed", reason: errorText(error) });
      },
    );
    return () => controller.abort();
  }, [attempt, id, loading]);

  const retry = useCallback(() => {
    setLoad({ kind: "loading" });
    setAttempt((value) => value + 1);
  }, []);
  return { load, setLoad, retry };
}

function loadingPanels() {
  const e = t.player.detailsEditor;
  const field = (label: UiText) => (
    <Field key={label}>
      <FieldTitle>{label}</FieldTitle>
      <Skeleton className="h-8 w-full" />
    </Field>
  );
  return [
    {
      key: "basics",
      legend: e.basics,
      content: [e.name, e.code, e.released, e.recorded, e.published, e.description].map(
        field,
      ),
    },
    {
      key: "credits",
      legend: e.credits,
      content: [
        e.performers,
        e.directors,
        e.photographers,
        e.streamers,
        e.makers,
        e.labels,
      ].map(field),
    },
  ];
}

/** Key the owner-only editor by video ID because React Router reuses route elements; stale drafts must never save to the previous video. */
export default function VideoDetailsEditor() {
  const id = parseId(useParams().id);
  return <DetailsEditor key={id} id={id} />;
}

function DetailsEditor({ id }: { id: number }) {
  const { load, setLoad, retry } = useVideoLoad(id);
  const [changed, setChanged] = useState(false);
  const [saving, setSaving] = useState(false);
  const backTo = `/videos/${String(id)}`;
  const leaving = useLeaving<Video>({
    pagePath: backTo,
    changed: load.kind === "ready" && changed,
    saving,
    handBack: (saved) => handBackToPage(id, saved),
  });
  const e = t.player.detailsEditor;
  const video = load.kind === "ready" ? load.video : undefined;

  const title = video?.title;
  useEffect(() => {
    const previous = document.title;
    if (title !== undefined) document.title = e.documentTitle(title);
    return () => {
      document.title = previous;
    };
  }, [e, title]);

  const confirm = (
    <ConfirmDialog
      open={leaving.confirmOpen}
      onOpenChange={(open) => {
        if (!open) leaving.keepEditing();
      }}
      title={e.discardTitle}
      description={e.discardText(title ?? "")}
      actionLabel={e.discard}
      cancelLabel={e.keepEditing}
      onConfirm={() => leaving.leave()}
    />
  );

  if (video !== undefined) {
    return (
      <>
        <DetailsForm
          key={video.id}
          video={video}
          backTo={backTo}
          onLeave={leaving.requestLeave}
          onSaved={(saved) => leaving.leave(saved)}
          onGone={() => {
            setChanged(false);
            setLoad({ kind: "gone" });
          }}
          onChangedChange={setChanged}
          onSavingChange={setSaving}
        />
        {confirm}
      </>
    );
  }

  const status =
    load.kind === "failed" ? (
      <ErrorState
        title={e.loadFailed(load.reason)}
        retryLabel={e.retry}
        onRetry={retry}
      />
    ) : load.kind === "gone" ? (
      <ErrorState title={e.gone} />
    ) : undefined;

  return (
    <FormPage
      title={e.title}
      actions={
        <EditorBand
          saving={false}
          saveDisabled
          onCancel={() => leaving.leave()}
          onSave={() => undefined}
        />
      }
      asideLabel={e.video}
      aside={
        <EditorAside video={undefined} backTo={backTo} onBack={() => leaving.leave()} />
      }
      panels={loadingPanels()}
      status={status}
    />
  );
}
