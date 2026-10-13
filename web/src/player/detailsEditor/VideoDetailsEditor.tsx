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

/** isSentinel は、今の履歴の項目が Back を受け止めるために足した項目かを返す。 */
function isSentinel(state: unknown): boolean {
  return (state as { sentinel?: unknown } | null)?.sentinel === true;
}

/** fromPage は、動画ページの入口から開いたか（下に動画ページの履歴があるか）を返す。 */
function openedFromPage(state: unknown): boolean {
  return (state as { fromPage?: unknown } | null)?.fromPage === true;
}

function parseId(raw: string | undefined): number {
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 0;
}

/**
 * useVideoLoad は編集画面の動画である。入口から開いたときはページの `Video` から始め、
 * URL で開いたときは `GET /api/videos/{id}` を読む（ui-design.md「Editor」の States）。
 */
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

/**
 * useLeaving は編集画面からの離れ方である（ui-design.md「Leaving」）。
 *
 * - 開いたら同じ URL の履歴の項目を 1 つ足し、ブラウザの Back をそこで受け止める。Back で
 *   下の項目へ戻ったら、下書きが変わっていなければそのまま離れ、変わっていれば確かめる
 *   （「Keep editing」なら項目を足し直す）。
 * - 離れるときは、入口から開いたなら履歴を動画ページまで戻し（ページの × の戻り先が残る）、
 *   URL で開いたなら `/videos/:id` へ置き換える。
 * - 下書きが変わっている間は、再読み込みと閉じるタブにブラウザの確かめを出す。
 */
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
  // URL で開いた編集画面を足した項目から離れるときは、下の編集画面の項目まで戻ってから
  // それを `/videos/:id` へ置き換える。置き換えるのは戻り着いた後である。
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
        // 足した項目だけを置き換えると、下の編集画面の項目が残り、動画ページの Back が
        // 編集画面を開き直してしまう。
        replaceOnArrival.current = true;
        void navigate(-1);
      } else {
        void navigate(`/videos/${String(id)}`, { replace: true });
      }
    },
    [id, navigate],
  );

  useEffect(() => {
    // StrictMode が効果を 2 度走らせても、同じ項目は 1 度だけ扱う。
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
    // Back で足した項目から戻ってきた。
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
      // 古いブラウザは returnValue を見る。
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
    // 「Discard」で離れたあとに確かめの窓が閉じても、履歴を足し直さない。
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

/** LoadingPanels は読み込みの間の、欄の名前と入力の形である（ui-design.md「States」）。 */
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

/**
 * VideoDetailsEditor は全画面の編集画面（`/videos/:id/edit`）で、表示名と作品情報のすべてを
 * 1 回の `Save` で直す（specs/047-video-work-info/ui-design.md「Editor」）。AppShell と
 * プレイヤーの外に置き、所有者だけが開ける（AuthGate の ownerOnlyPath）。
 *
 * 経路の id が変わっても React Router は同じ要素を使い続けるので、id ごとに作り直す。
 * そうしないと読み込み・下書き・離れ方が前の動画のまま残り、Save が前の動画へ書く。
 */
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
