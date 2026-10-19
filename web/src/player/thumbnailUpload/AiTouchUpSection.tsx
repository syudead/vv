import { LoaderCircle, WandSparkles } from "lucide-react";
import { useState } from "react";

import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import { Progress } from "../../ui/shadcn/progress";
import { Textarea } from "../../ui/shadcn/textarea";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
import { isRunning, runStateText } from "../thumbnailEdit/editRules";
import { PRESETS } from "../thumbnailEdit/presets";
import type { TouchUp } from "./useAiTouchUp";

const PROMPT_MAX = 4000;

/** ComfyUI touch-ups of the current framing: fixed instructions or the owner's own, then a before/after choice. */
export default function AiTouchUpSection({
  state,
  after,
  disabled,
  onRun,
  onCancel,
  onShowAfter,
  onUse,
  onDismiss,
}: {
  state: TouchUp;
  after: boolean;
  disabled: boolean;
  onRun: (prompt: string) => void;
  onCancel: () => void;
  onShowAfter: (after: boolean) => void;
  onUse: () => void;
  onDismiss: () => void;
}) {
  const text = t.player.thumbnailUpload.ai;
  const [prompt, setPrompt] = useState("");
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm font-medium">{text.title}</p>
      {state.status === "running" && <Running state={state} onCancel={onCancel} />}
      {state.status === "result" && (
        <>
          <ToggleGroup
            type="single"
            variant="outline"
            size="default"
            aria-label={text.compare}
            className="w-full"
            value={after ? "after" : "before"}
            onValueChange={(value) => {
              if (value !== "") onShowAfter(value === "after");
            }}
          >
            <ToggleGroupItem value="before">{text.before}</ToggleGroupItem>
            <ToggleGroupItem value="after">{text.after}</ToggleGroupItem>
          </ToggleGroup>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" size="sm" onClick={onUse}>
              {text.use}
            </Button>
            <Button variant="ghost" size="sm" onClick={onDismiss}>
              {text.dismiss}
            </Button>
          </div>
          <p className="text-sm text-muted-foreground">{text.resultHint}</p>
        </>
      )}
      {state.status !== "running" && state.status !== "result" && (
        <>
          <div className="flex flex-wrap gap-2">
            {PRESETS.map((preset) => (
              <Button
                key={preset.id}
                variant="outline"
                size="sm"
                disabled={disabled}
                onClick={() => onRun(preset.prompt)}
              >
                <WandSparkles aria-hidden="true" />
                {t.player.thumbnailEdit.presets[preset.id]}
              </Button>
            ))}
          </div>
          <Textarea
            aria-label={text.own}
            rows={2}
            maxLength={PROMPT_MAX}
            placeholder={text.ownPlaceholder}
            value={prompt}
            disabled={disabled}
            onChange={(event) => setPrompt(event.target.value)}
          />
          <Button
            variant="outline"
            size="sm"
            className="self-start"
            disabled={disabled || prompt.trim() === ""}
            onClick={() => onRun(prompt.trim())}
          >
            {text.runOwn}
          </Button>
          {state.status === "failed" && (
            <p role="alert" className="text-sm text-destructive">
              {state.message}
            </p>
          )}
          {state.status === "cancelled" && (
            <p className="text-sm text-muted-foreground">{text.cancelled}</p>
          )}
          <p className="text-sm text-muted-foreground">{text.hint}</p>
        </>
      )}
    </div>
  );
}

function Running({
  state,
  onCancel,
}: {
  state: Extract<TouchUp, { status: "running" }>;
  onCancel: () => void;
}) {
  const text = t.player.thumbnailEdit;
  const session = state.session;
  const label =
    session !== null && isRunning(session.state)
      ? runStateText(session.state)
      : t.player.thumbnailUpload.ai.starting;
  const progress = session?.progress;
  return (
    <div role="status" className="flex flex-col gap-2">
      <p className="flex items-center gap-2 text-sm">
        <LoaderCircle
          aria-hidden="true"
          className="size-4 animate-spin motion-reduce:animate-none"
        />
        {label}
      </p>
      {progress !== undefined && progress.max > 0 && (
        <>
          <Progress
            aria-label={text.progress}
            value={(progress.value / progress.max) * 100}
          />
          <p className="text-sm text-muted-foreground tabular-nums">
            {text.step(progress.value, progress.max)}
          </p>
        </>
      )}
      <Button variant="outline" size="sm" className="self-start" onClick={onCancel}>
        {t.player.thumbnailUpload.ai.stop}
      </Button>
    </div>
  );
}
