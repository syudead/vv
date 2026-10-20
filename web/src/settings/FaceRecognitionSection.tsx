import { Unplug } from "lucide-react";
import { useState } from "react";

import {
  downloadFaceRuntime,
  type FaceRecognitionSettings,
  startFaceRecognition,
  updateFaceRecognitionSettings,
} from "../api/client";
import { errorText, t } from "../i18n";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Switch } from "../ui/shadcn/switch";
import { useToast } from "../ui/Toast";

import { FaceRecognizerChoice, useRecognizerSave } from "./FaceRecognizerChoice";
import { FaceRuntimeLine } from "./FaceRuntimeLine";
import {
  RunProgressSection,
  RunSectionsLoading,
  useRunStart,
} from "./RunProgressSection";
import { SaveStatus } from "./SaveStatus";
import type { FaceRecognitionSource } from "./useFaceRecognitionSettings";
import { useSwitchSave } from "./useSwitchSave";

/** The page notice while no face runtime exists for this platform; it locks every control. */
export function faceRecognitionNotice(settings: FaceRecognitionSettings | null) {
  if (settings?.runtime.state !== "unsupported") return undefined;
  const text = t.settings.faceRecognition;
  return (
    <Alert role="note">
      <Unplug aria-hidden="true" />
      <AlertTitle>{text.unsupportedTitle}</AlertTitle>
      <AlertDescription>
        <p>{text.unsupportedBody}</p>
      </AlertDescription>
    </Alert>
  );
}

function useDownloadAgain(source: FaceRecognitionSource, locked: boolean) {
  const toast = useToast();
  const [retrying, setRetrying] = useState(false);
  const retry = async () => {
    if (locked || retrying) return;
    setRetrying(true);
    source.beginSave();
    try {
      source.saved(await downloadFaceRuntime());
    } catch (failure) {
      toast(t.settings.faceRecognition.downloadFailed(errorText(failure)));
    } finally {
      source.endSave();
      setRetrying(false);
    }
  };
  return { retrying, retry };
}

function RecognitionSection({
  source,
  settings,
  locked,
}: {
  source: FaceRecognitionSource;
  settings: FaceRecognitionSettings;
  locked: boolean;
}) {
  const text = t.settings.faceRecognition;
  const toggleSave = useSwitchSave({
    settings,
    current: (saved) => saved.enabled,
    save: (enabled) =>
      updateFaceRecognitionSettings({ enabled, recognizer: settings.recognizer }),
    saved: source.saved,
    saveFailed: text.saveFailed,
    onStart: source.beginSave,
    onSettled: source.endSave,
  });
  const switchBusy = toggleSave.saving !== null;
  const download = useDownloadAgain(source, locked);
  // Each save and the retry answer with the whole settings, so one waits for the others.
  const recognizer = useRecognizerSave(source, locked || switchBusy || download.retrying);
  const recognizerBusy = recognizer.saving;

  return (
    <PageSection title={text.heading} description={text.description}>
      <div className="flex flex-col gap-2">
        <FormRow
          label={text.enabled}
          description={text.enabledHint}
          htmlFor="face-recognition-enabled"
        >
          <Switch
            id="face-recognition-enabled"
            checked={toggleSave.saving ?? settings.enabled}
            aria-disabled={
              locked || switchBusy || recognizerBusy || download.retrying || undefined
            }
            onCheckedChange={() => {
              if (!locked && !recognizerBusy && !download.retrying)
                void toggleSave.toggle();
            }}
          />
        </FormRow>
        {settings.enabled && (
          <FaceRuntimeLine
            runtime={settings.runtime}
            retrying={download.retrying}
            locked={locked || switchBusy || recognizerBusy}
            onRetry={() => {
              if (!switchBusy && !recognizerBusy) void download.retry();
            }}
          />
        )}
      </div>
      <SaveStatus
        saving={switchBusy}
        savingText={text.saving}
        error={toggleSave.saveError}
      />
      <FaceRecognizerChoice
        settings={settings}
        save={recognizer}
        disabled={locked || switchBusy || recognizer.saving || download.retrying}
      />
      <SaveStatus
        saving={recognizer.saving && recognizer.confirming === null}
        savingText={text.saving}
        error={recognizer.saveError}
      />
    </PageSection>
  );
}

/**
 * The Progress and Recognition sections of the Face recognition page. While unsupported every
 * control keeps its value with aria-disabled and each handler ignores the press.
 */
export function FaceRecognitionSections({ source }: { source: FaceRecognitionSource }) {
  const text = t.settings.faceRecognition;
  const { settings, loadError, load } = source;
  const locked = settings?.runtime.state === "unsupported";
  const { starting, run } = useRunStart({
    start: () => startFaceRecognition("missing"),
    started: text.started,
    startFailed: text.startFailed,
    reload: load,
    locked,
  });

  if (settings === null)
    return (
      <RunSectionsLoading
        progressHeading={text.progressHeading}
        heading={text.heading}
        description={text.description}
        loadingLabel={text.loading}
        loadError={loadError === null ? null : text.loadFailed(loadError)}
        onRetry={() => void load()}
      />
    );

  return (
    <>
      <RunProgressSection
        text={text}
        enabled={settings.enabled}
        counts={settings.counts}
        lastFailure={settings.lastFailure}
        starting={starting}
        locked={locked}
        onStart={(scope) => void run(scope)}
      />
      <RecognitionSection source={source} settings={settings} locked={locked} />
    </>
  );
}
