import {
  ImagePlus,
  LoaderCircle,
  Paintbrush,
  Eraser,
  Trash2,
  Undo2,
  X,
} from "lucide-react";
import {
  type ChangeEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useReducer,
  useRef,
  useState,
} from "react";

import {
  cancelThumbnailEdit,
  discardThumbnailEdit,
  getThumbnailEditResult,
  getThumbnailEdit,
  isAborted,
  RequestFailed,
  runThumbnailEdit,
  type ThumbnailEdit,
} from "../../api/client";
import { errorText, t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { EditorDialog } from "../../ui/patterns/editor-dialog";
import { Alert, AlertDescription, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Field, FieldDescription, FieldLabel, FieldTitle } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import { ImageDropOverlay, useImageIntake } from "../../ui/ImageDrop";
import { Progress } from "../../ui/shadcn/progress";
import { Slider } from "../../ui/shadcn/slider";
import { Spinner } from "../../ui/shadcn/spinner";
import { Textarea } from "../../ui/shadcn/textarea";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import {
  BRUSH_STEPS,
  brushDiameter,
  elapsedNow,
  encodeReferenceURL,
  isRunning,
  referenceAccepted,
  runStateText,
  shapeDiffers,
  showsLongWait,
  sourceWords,
} from "./editRules";
import {
  type EditorState,
  editorReducer,
  hasResult,
  initialEditorState,
  needsConfirmation,
} from "./editSession";
import CompareStage from "./CompareStage";
import { buildMask, type Stroke } from "./mask";
import PaintStage from "./PaintStage";
import { PRESETS } from "./presets";

const RUN_POLL_MS = 1000;
const IDLE_POLL_MS = 60_000;

const PROMPT_MAX = 4000;

function sessionEnded(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.reason === "thumbnail_edit_not_found"
  );
}

/** Provider failure messages live on the failed session, not the start response; reload to retrieve them. */
async function providerMessage(
  editId: string,
  error: unknown,
): Promise<string | undefined> {
  if (!(error instanceof RequestFailed) || error.code !== "provider_failed")
    return undefined;
  try {
    const failed = await getThumbnailEdit(editId);
    return failed.state === "failed" ? failed.error?.message : undefined;
  } catch {
    return undefined;
  }
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);
  return now;
}

/** Polling doubles as a heartbeat for the five-minute session expiry. Pause around mutations to reject stale reads, then resume even after failure. */
function useSessionPolling(
  state: EditorState,
  dispatch: (action: Parameters<typeof editorReducer>[1]) => void,
): { pause: () => void; resume: () => void } {
  const epoch = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const [retry, setRetry] = useState(0);
  const { session, ended } = state;
  const running = isRunning(session.state);

  useEffect(() => {
    if (ended) return;
    const sentEpoch = epoch.current;
    const timer = window.setTimeout(
      () => {
        const request = new AbortController();
        controller.current = request;
        getThumbnailEdit(session.id, request.signal).then(
          (next) => {
            if (request.signal.aborted || epoch.current !== sentEpoch) return;
            dispatch({ type: "read", session: next, at: Date.now() });
          },
          (error: unknown) => {
            if (isAborted(error) || request.signal.aborted) return;
            if (epoch.current !== sentEpoch) return;
            if (sessionEnded(error)) dispatch({ type: "ended" });
            else setRetry((count) => count + 1);
          },
        );
      },
      running ? RUN_POLL_MS : IDLE_POLL_MS,
    );
    return () => window.clearTimeout(timer);
  }, [dispatch, ended, running, session, state.receivedAt, retry]);

  useEffect(() => () => controller.current?.abort(), []);

  const pause = useCallback(() => {
    epoch.current += 1;
    controller.current?.abort();
  }, []);
  const resume = useCallback(() => setRetry((count) => count + 1), []);
  return { pause, resume };
}

/** Discard sessions when leaving before apply/close, excluding StrictMode effect replay. */
function useDiscardOnLeave(editId: string, settled: { current: boolean }) {
  const pending = useRef<number | null>(null);
  useEffect(() => {
    if (pending.current !== null) {
      window.clearTimeout(pending.current);
      pending.current = null;
    }
    const discard = () => {
      if (settled.current) return;
      settled.current = true;
      void discardThumbnailEdit(editId).catch(() => undefined);
    };
    window.addEventListener("pagehide", discard);
    return () => {
      window.removeEventListener("pagehide", discard);
      pending.current = window.setTimeout(discard, 0);
    };
  }, [editId, settled]);
}

/** The result goes back to the caller, which frames it before it becomes the thumbnail. */
export default function ThumbnailEditor({
  edit,
  onUseResult,
  onClosed,
}: {
  edit: ThumbnailEdit;
  onUseResult: (result: Blob) => void;
  onClosed: () => void;
}) {
  const text = t.player.thumbnailEdit;
  const [state, dispatch] = useReducer(editorReducer, edit, (session) =>
    initialEditorState(session, Date.now()),
  );
  const { session, view, ended } = state;
  const editId = session.id;
  // The session response fixes the source image for the lifetime of the editor.
  const words = sourceWords(edit);
  const running = isRunning(session.state);
  const polling = useSessionPolling(state, dispatch);
  const settled = useRef(false);
  useDiscardOnLeave(editId, settled);

  const [prompt, setPrompt] = useState("");
  const [strokes, setStrokes] = useState<Stroke[]>([]);
  const [mode, setMode] = useState<Stroke["mode"]>("paint");
  const [brushStep, setBrushStep] = useState(1);
  const [reference, setReference] = useState<{ file: File; url: string } | null>(null);
  const [referenceRefused, setReferenceRefused] = useState(false);
  const [busy, setBusy] = useState<"generate" | "cancel" | "apply" | null>(null);
  const [applyFailure, setApplyFailure] = useState<UiText | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [resultShape, setResultShape] = useState<{
    url: string;
    differs: boolean;
  } | null>(null);
  const instructions = useRef<HTMLTextAreaElement | null>(null);
  const fileInput = useRef<HTMLInputElement | null>(null);
  const now = useNow(running && !ended);

  useEffect(
    () => () => {
      if (reference !== null) URL.revokeObjectURL(reference.url);
    },
    [reference],
  );

  const close = () => {
    if (!settled.current) {
      settled.current = true;
      polling.pause();
      void discardThumbnailEdit(editId).catch(() => undefined);
    }
    onClosed();
  };

  const requestClose = () => {
    if (busy === "apply") return;
    if (needsConfirmation(state)) setConfirming(true);
    else close();
  };

  const ended404 = (error: unknown): boolean => {
    if (!sessionEnded(error)) return false;
    dispatch({ type: "ended" });
    return true;
  };

  const generate = async () => {
    if (busy !== null || ended || running) return;
    polling.pause();
    setBusy("generate");
    setApplyFailure(null);
    try {
      const mask = await buildMask(strokes, session.frameWidth, session.frameHeight);
      const next = await runThumbnailEdit(editId, {
        prompt: prompt.trim() === "" ? undefined : prompt,
        mask,
        reference: reference?.file,
      });
      dispatch({ type: "started", session: next, at: Date.now() });
    } catch (error) {
      if (!ended404(error))
        dispatch({
          type: "rejected",
          failure: {
            title: text.runFailed(errorText(error)),
            message: await providerMessage(editId, error),
          },
        });
    } finally {
      setBusy(null);
      polling.resume();
    }
  };

  const cancel = async () => {
    if (busy !== null || ended) return;
    polling.pause();
    setBusy("cancel");
    try {
      const next = await cancelThumbnailEdit(editId);
      dispatch({ type: "read", session: next, at: Date.now() });
    } catch (error) {
      // If cancellation fails, generation continues and polling determines the next state.
      ended404(error);
    } finally {
      setBusy(null);
      polling.resume();
    }
  };

  const apply = async () => {
    if (busy !== null || ended || !hasResult(state)) return;
    polling.pause();
    setBusy("apply");
    setApplyFailure(null);
    try {
      const result = await getThumbnailEditResult(editId);
      settled.current = true;
      void discardThumbnailEdit(editId).catch(() => undefined);
      onUseResult(result);
      onClosed();
    } catch (error) {
      setApplyFailure(text.applyFailed(errorText(error)));
      if (
        sessionEnded(error) ||
        (error instanceof RequestFailed && error.reason === "content_changed")
      ) {
        settled.current = true;
        dispatch({ type: "ended" });
      }
    } finally {
      setBusy(null);
      if (!settled.current) polling.resume();
    }
  };

  const chooseReference = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file !== undefined) takeReference(file);
  };

  const takeReference = (file: File) => {
    if (!referenceAccepted(file)) {
      setReferenceRefused(true);
      return;
    }
    setReferenceRefused(false);
    setReference({ file, url: URL.createObjectURL(file) });
  };

  const locked = running || ended || busy !== null || view === "compare";
  const inputsLocked = running || ended || busy !== null || view === "compare";
  const { dragging } = useImageIntake({
    enabled: !inputsLocked && !confirming,
    onFile: takeReference,
    scope: "dialog",
  });
  const resultUrl = session.resultUrl;
  const elapsedMs =
    session.elapsedMs === undefined
      ? undefined
      : running
        ? elapsedNow(session.elapsedMs, state.receivedAt, now)
        : session.elapsedMs;

  const stage =
    view === "compare" && resultUrl !== undefined ? (
      <CompareStage
        frameUrl={session.frameUrl}
        resultUrl={resultUrl}
        sourceLabel={words.sourceLabel}
        aspect={session.frameWidth / session.frameHeight}
        cropNote={resultShape?.url === resultUrl && resultShape.differs}
        onResultSize={(width, height) =>
          setResultShape({
            url: resultUrl,
            differs: shapeDiffers(session.frameWidth, session.frameHeight, width, height),
          })
        }
      />
    ) : (
      <div className="flex size-full min-h-0 flex-col gap-3">
        <div
          role="toolbar"
          aria-label={text.tools}
          className="flex flex-wrap items-center gap-2"
        >
          <ToggleGroup
            type="single"
            variant="outline"
            size="default"
            value={mode}
            disabled={locked}
            onValueChange={(value) => {
              if (value === "paint" || value === "erase") setMode(value);
            }}
          >
            <ToggleGroupItem value="paint">
              <Paintbrush aria-hidden="true" />
              {text.paint}
            </ToggleGroupItem>
            <ToggleGroupItem value="erase">
              <Eraser aria-hidden="true" />
              {text.erase}
            </ToggleGroupItem>
          </ToggleGroup>
          <Slider
            aria-label={text.brushSize}
            min={0}
            max={BRUSH_STEPS.length - 1}
            step={1}
            value={[brushStep]}
            disabled={locked}
            onValueChange={([value]) => setBrushStep(value ?? 1)}
            className="w-zoom"
          />
          <ToolButton
            label={text.undo}
            disabled={locked || strokes.length === 0}
            onClick={() => setStrokes((previous) => previous.slice(0, -1))}
          >
            <Undo2 aria-hidden="true" />
          </ToolButton>
          <ToolButton
            label={text.clearPainting}
            disabled={locked || strokes.length === 0}
            onClick={() => setStrokes([])}
          >
            <Trash2 aria-hidden="true" />
          </ToolButton>
        </div>
        <div className="flex min-h-0 flex-1">
          <PaintStage
            frameUrl={session.frameUrl}
            frameWidth={session.frameWidth}
            frameHeight={session.frameHeight}
            strokes={strokes}
            mode={mode}
            brushSize={brushDiameter(brushStep, session.frameWidth, session.frameHeight)}
            locked={locked}
            onStroke={(stroke) => setStrokes((previous) => [...previous, stroke])}
            onUndo={() => setStrokes((previous) => previous.slice(0, -1))}
          />
        </div>
        <p className="text-sm text-muted-foreground">{words.paintingHint}</p>
      </div>
    );

  const aside = (
    <>
      <Field>
        <FieldLabel htmlFor="thumbnail-edit-instructions">{text.instructions}</FieldLabel>
        <Textarea
          id="thumbnail-edit-instructions"
          ref={instructions}
          rows={4}
          maxLength={PROMPT_MAX}
          placeholder={text.instructionsPlaceholder}
          value={prompt}
          disabled={inputsLocked}
          onChange={(event) => setPrompt(event.target.value)}
        />
        <div role="group" aria-label={text.presetsLabel} className="flex flex-wrap gap-2">
          {PRESETS.map((preset) => (
            <Button
              key={preset.id}
              variant="outline"
              size="sm"
              disabled={inputsLocked}
              onClick={() => setPrompt(preset.prompt)}
            >
              {text.presets[preset.id]}
            </Button>
          ))}
        </div>
        <FieldDescription>{text.instructionsHint}</FieldDescription>
      </Field>
      <Field>
        <FieldTitle>{text.reference}</FieldTitle>
        <Input
          ref={fileInput}
          type="file"
          accept="image/png,image/jpeg,image/webp"
          tabIndex={-1}
          aria-hidden="true"
          className="hidden"
          onChange={chooseReference}
        />
        {reference === null ? (
          <Button
            variant="outline"
            className="w-fit"
            disabled={inputsLocked}
            onClick={() => fileInput.current?.click()}
          >
            <ImagePlus aria-hidden="true" />
            {text.addReference}
          </Button>
        ) : (
          <div className="flex min-w-0 items-center gap-2">
            <img
              src={encodeReferenceURL(reference.url)}
              alt=""
              className="h-16 w-auto shrink-0 rounded-sm object-contain"
            />
            <span className="min-w-0 flex-1 truncate text-sm">{reference.file.name}</span>
            <ToolButton
              label={text.removeReference}
              size="icon-sm"
              disabled={inputsLocked}
              onClick={() => setReference(null)}
            >
              <X aria-hidden="true" />
            </ToolButton>
          </div>
        )}
        {referenceRefused && (
          <Alert variant="destructive">
            <AlertTitle>{text.referenceRefused}</AlertTitle>
          </Alert>
        )}
        <FieldDescription>{text.referenceHint}</FieldDescription>
      </Field>
      <RunArea state={state} elapsedMs={elapsedMs} />
      {ended ? (
        <Alert variant="destructive">
          <AlertTitle>
            {applyFailure ?? t.errors.reason.thumbnail_edit_not_found}
          </AlertTitle>
        </Alert>
      ) : (
        applyFailure !== null && (
          <Alert variant="destructive">
            <AlertTitle>{applyFailure}</AlertTitle>
          </Alert>
        )
      )}
    </>
  );

  let footer: ReactNode;
  if (view === "running") {
    footer = (
      <Button
        variant="outline"
        size="lg"
        className="w-full"
        disabled={ended}
        aria-disabled={busy === "cancel" || undefined}
        aria-busy={busy === "cancel" || undefined}
        onClick={() => void cancel()}
      >
        {busy === "cancel" && <Spinner aria-hidden="true" role={undefined} />}
        {text.cancel}
      </Button>
    );
  } else if (view === "compare") {
    footer = (
      <>
        <Button
          size="lg"
          className="w-full"
          disabled={ended}
          aria-disabled={busy !== null || undefined}
          aria-busy={busy === "apply" || undefined}
          onClick={() => void apply()}
        >
          {busy === "apply" && (
            <LoaderCircle
              aria-hidden="true"
              className="animate-spin motion-reduce:animate-none"
            />
          )}
          {busy === "apply" ? text.applying : t.player.thumbnailUpload.ai.use}
        </Button>
        <div className="flex gap-2">
          <Button
            variant="outline"
            className="min-w-0 flex-1"
            disabled={ended || busy !== null}
            onClick={() => dispatch({ type: "change" })}
          >
            {text.changeAndGenerate}
          </Button>
          <Button
            variant="ghost"
            className="shrink-0"
            disabled={ended || busy !== null}
            onClick={close}
          >
            {text.discard}
          </Button>
        </div>
      </>
    );
  } else {
    footer = (
      <>
        {hasResult(state) && (
          <Button
            variant="ghost"
            className="w-full"
            disabled={busy !== null}
            onClick={() => dispatch({ type: "showResult" })}
          >
            {text.showResult}
          </Button>
        )}
        <Button
          size="lg"
          className="w-full"
          disabled={ended}
          aria-disabled={busy !== null || undefined}
          aria-busy={busy === "generate" || undefined}
          onClick={() => void generate()}
        >
          {busy === "generate" && (
            <LoaderCircle
              aria-hidden="true"
              className="animate-spin motion-reduce:animate-none"
            />
          )}
          {state.ran ? text.generateAgain : text.generate}
        </Button>
      </>
    );
  }

  return (
    <>
      <EditorDialog
        open
        onClose={requestClose}
        title={text.title}
        detail={words.detail}
        closeLabel={text.close}
        closeDisabled={busy === "apply"}
        initialFocus={instructions}
        stage={stage}
        aside={aside}
        footer={footer}
      />
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={text.confirmTitle}
        description={text.confirmDescription}
        actionLabel={text.discard}
        cancelLabel={text.keepEditing}
        onConfirm={() => {
          setConfirming(false);
          close();
        }}
      />
      <ImageDropOverlay dragging={dragging} label={text.dropReference} />
    </>
  );
}

function ToolButton({
  label,
  size = "icon",
  disabled,
  onClick,
  children,
}: {
  label: UiText;
  size?: "icon" | "icon-sm";
  disabled: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size={size}
          aria-label={label}
          disabled={disabled}
          onClick={onClick}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function RunArea({
  state,
  elapsedMs,
}: {
  state: EditorState;
  elapsedMs: number | undefined;
}) {
  const text = t.player.thumbnailEdit;
  const { session, view, failure, cancelled, ended } = state;
  const elapsed =
    elapsedMs === undefined ? null : (
      <span className="ml-auto text-muted-foreground tabular-nums">
        <span className="sr-only">{text.elapsed} </span>
        {formatDuration(elapsedMs)}
      </span>
    );
  let status: ReactNode = null;
  if (view === "running" && isRunning(session.state)) {
    status = (
      <>
        <div className="flex items-center gap-2">
          <Spinner aria-hidden="true" role={undefined} />
          <span className="text-base font-medium">{runStateText(session.state)}</span>
          {elapsed}
        </div>
        {session.progress !== undefined && (
          <div className="flex flex-col gap-1">
            <Progress
              aria-label={text.progress}
              value={session.progress.value}
              max={session.progress.max}
            />
            <span className="text-sm text-muted-foreground">
              {text.step(session.progress.value, session.progress.max)}
            </span>
          </div>
        )}
        {showsLongWait(session.state, elapsedMs ?? 0) && (
          <p className="text-sm text-muted-foreground">{text.longWait}</p>
        )}
      </>
    );
  } else if (view === "compare") {
    status = elapsed !== null && (
      <div className="flex items-center text-sm">{elapsed}</div>
    );
  } else if (cancelled && !ended) {
    status = <p className="text-sm text-muted-foreground">{text.cancelled}</p>;
  }
  return (
    <>
      <div role="status" className="flex flex-col gap-2 empty:hidden">
        {status}
      </div>
      {view === "compose" && failure !== null && !ended && (
        <Alert variant="destructive">
          <AlertTitle>{failure.title}</AlertTitle>
          {failure.message !== undefined && failure.message !== "" && (
            <AlertDescription>{failure.message}</AlertDescription>
          )}
        </Alert>
      )}
    </>
  );
}
