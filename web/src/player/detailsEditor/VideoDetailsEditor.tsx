import { useCallback, useEffect, useRef, useState } from "react";
import { type Location, useLocation, useNavigate, useParams } from "react-router";

import { getVideo, isAborted, RequestFailed, type Video } from "../../api/client";
import { errorText, t, type UiText } from "../../i18n";
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { ErrorState } from "../../ui/patterns/error-state";
import { FormPage } from "../../ui/patterns/form-page";
import { Field, FieldTitle } from "../../ui/shadcn/field";
import { Skeleton } from "../../ui/shadcn/skeleton";
import DetailsForm, { EditorBand } from "./DetailsForm";
import EditorAside from "./EditorAside";
import { clearVideoForEditor, handBackToPage, peekVideoForEditor } from "./handoff";

type Load =
  | { kind: "loading" }
  | { kind: "ready"; video: Video }
  | { kind: "failed"; reason: UiText }
  | { kind: "gone" };

function isSentinel(state: unknown): boolean {
  return (state as { sentinel?: unknown } | null)?.sentinel === true;
}

function openedFromPage(state: unknown): boolean {
  return (state as { fromPage?: unknown } | null)?.fromPage === true;
}

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

/** Add a history sentinel to intercept Back and confirm dirty drafts. Restore the video page's history or replace direct editor URLs on exit. */
function useLeaving(id: number, changed: boolean, saving: boolean) {
  const location = useLocation();
  const navigate = useNavigate();
  const locationRef = useRef<Location>(location);
  locationRef.current = location;
  const changedRef = useRef(changed);
  changedRef.current = changed;
  const savingRef = useRef(saving);
  savingRef.current = saving;
  const leaving = useRef(false);
  // Pop the sentinel before replacing a directly opened editor; otherwise Back would reopen the leftover editor entry.
  const replaceOnArrival = useRef(false);
  const sentinelSeen = useRef(false);
  const handledKey = useRef<string | null>(null);
  const [confirm, setConfirm] = useState<"cancel" | "back" | null>(null);

  const pushSentinel = useCallback(() => {
    const current = locationRef.current;
    const state = (current.state ?? {}) as Record<string, unknown>;
    void navigate(
      { pathname: current.pathname, search: current.search },
      { state: { ...state, sentinel: true } },
    );
  }, [navigate]);

  const leave = useCallback(
    (video?: Video) => {
      leaving.current = true;
      setConfirm(null);
      handBackToPage(id, video);
      const onSentinel = isSentinel(locationRef.current.state);
      if (openedFromPage(locationRef.current.state)) {
        void navigate(onSentinel ? -2 : -1);
      } else if (onSentinel) {
        replaceOnArrival.current = true;
        void navigate(-1);
      } else {
        void navigate(`/videos/${String(id)}`, { replace: true });
      }
    },
    [id, navigate],
  );

  useEffect(() => {
    // Process each history entry once despite StrictMode effect replay.
    if (handledKey.current === location.key) return;
    handledKey.current = location.key;
    if (isSentinel(location.state)) {
      sentinelSeen.current = true;
      return;
    }
    if (!sentinelSeen.current) {
      pushSentinel();
      return;
    }
    if (replaceOnArrival.current) {
      replaceOnArrival.current = false;
      void navigate(`/videos/${String(id)}`, { replace: true });
      return;
    }
    if (leaving.current) return;
    if (savingRef.current) {
      pushSentinel();
      return;
    }
    if (changedRef.current) {
      setConfirm("back");
      return;
    }
    leave();
  }, [id, leave, location.key, location.state, navigate, pushSentinel]);

  useEffect(() => {
    if (!changed) return;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      // Older browsers inspect returnValue.
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [changed]);

  const requestLeave = useCallback(() => {
    if (savingRef.current) return;
    if (changedRef.current) setConfirm("cancel");
    else leave();
  }, [leave]);

  const keepEditing = useCallback(() => {
    // Closing the discard dialog after leaving must not recreate the sentinel.
    if (leaving.current) return;
    if (confirm === "back") pushSentinel();
    setConfirm(null);
  }, [confirm, pushSentinel]);

  return {
    confirmOpen: confirm !== null,
    leave,
    requestLeave,
    keepEditing,
  };
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
  const leaving = useLeaving(id, load.kind === "ready" && changed, saving);
  const e = t.player.detailsEditor;
  const video = load.kind === "ready" ? load.video : undefined;
  const backTo = `/videos/${String(id)}`;

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
