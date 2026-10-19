import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useState } from "react";

import {
  type FaceRecognitionSettings,
  type FaceRecognizer,
  updateFaceRecognitionSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { formatBytes } from "../lib/format";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { FormRow } from "../ui/patterns/form-row";
import { Badge } from "../ui/shadcn/badge";
import { Label } from "../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../ui/shadcn/radio-group";

import type { FaceRecognitionSource } from "./useFaceRecognitionSettings";

export interface RecognizerSave {
  /** The recognizer the confirm dialog offers; null while it is closed. */
  confirming: FaceRecognizer | null;
  saving: boolean;
  saveError: UiText | null;
  choose: (recognizer: FaceRecognizer) => void;
  confirm: () => void;
  cancel: () => void;
}

/**
 * The radio shows the saved recognizer only, so it moves when the server answers. While the feature
 * is on a change confirms first, because it queues every video again.
 */
export function useRecognizerSave(
  source: FaceRecognitionSource,
  blocked: boolean,
): RecognizerSave {
  const [confirming, setConfirming] = useState<FaceRecognizer | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  const { settings } = source;

  const save = async (recognizer: FaceRecognizer) => {
    if (settings === null) return;
    source.beginSave();
    setSaving(true);
    setSaveError(null);
    try {
      source.saved(
        await updateFaceRecognitionSettings({ enabled: settings.enabled, recognizer }),
      );
    } catch (failure) {
      setSaveError(t.settings.faceRecognition.saveFailed(errorText(failure)));
    } finally {
      source.endSave();
      setSaving(false);
      setConfirming(null);
    }
  };

  const choose = (recognizer: FaceRecognizer) => {
    if (blocked || saving || settings === null || recognizer === settings.recognizer)
      return;
    if (settings.enabled) setConfirming(recognizer);
    else void save(recognizer);
  };

  return {
    confirming,
    saving,
    saveError,
    choose,
    confirm: () => {
      if (confirming !== null && !saving) void save(confirming);
    },
    cancel: () => {
      if (!saving) setConfirming(null);
    },
  };
}

function RecognizerOption({
  option,
  disabled,
}: {
  option: FaceRecognitionSettings["recognizers"][number];
  disabled: boolean;
}) {
  const text = t.settings.faceRecognition;
  const itemId = `face-recognizer-${option.id}`;
  const lineId = `face-recognizer-line-${option.id}`;
  const license = option.nonCommercial ? text.nonCommercialLicense : option.license;
  return (
    <div className="flex min-w-0 items-start gap-3">
      <RadioGroupItem
        id={itemId}
        value={option.id}
        aria-describedby={lineId}
        aria-disabled={disabled || undefined}
        className="mt-0.5"
      />
      <div className="flex min-w-0 flex-col gap-1">
        <Label htmlFor={itemId} className="flex-wrap font-normal">
          {text.recognizerName[option.id]}
          {option.nonCommercial && (
            <Badge variant="warning">
              <TriangleAlert aria-hidden="true" />
              {text.nonCommercial}
            </Badge>
          )}
        </Label>
        <p id={lineId} className="text-xs text-muted-foreground">
          {text.optionLine(license, formatBytes(option.downloadBytes))}
        </p>
      </div>
    </div>
  );
}

export function FaceRecognizerChoice({
  settings,
  save,
  disabled,
}: {
  settings: FaceRecognitionSettings;
  save: RecognizerSave;
  disabled: boolean;
}) {
  const text = t.settings.faceRecognition;
  const target = save.confirming;
  const targetName = target === null ? "" : text.recognizerName[target];
  const nonCommercial =
    settings.recognizers.find((option) => option.id === target)?.nonCommercial ?? false;
  return (
    <>
      <FormRow
        label={text.recognizer}
        description={text.recognizerHint}
        descriptionId="face-recognizer-hint"
        htmlFor="face-recognizer"
      >
        <RadioGroup
          id="face-recognizer"
          aria-label={text.recognizer}
          aria-describedby="face-recognizer-hint"
          value={settings.recognizer}
          onValueChange={(value) => save.choose(value as FaceRecognizer)}
        >
          {settings.recognizers.map((option) => (
            <RecognizerOption key={option.id} option={option} disabled={disabled} />
          ))}
        </RadioGroup>
      </FormRow>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) save.cancel();
        }}
        title={text.switchTitle(targetName)}
        description={text.switchBody(targetName, nonCommercial)}
        actionLabel={
          <>
            {save.saving && (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            )}
            {text.switchAction}
          </>
        }
        cancelLabel={t.common.cancel}
        pending={save.saving}
        onConfirm={save.confirm}
      />
    </>
  );
}
