import { useCallback, useEffect, useRef, useState } from "react";

import {
  getVersionJudgingSettings,
  isAborted,
  updateVersionJudgingSettings,
  type VersionJudgingSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Switch } from "../ui/shadcn/switch";

import { PageLinkText } from "./PageLink";
import { SaveStatus } from "./SaveStatus";
import { useOllamaConnection } from "./useOllamaConnection";
import { useSwitchSave } from "./useSwitchSave";

/**
 * The switch remains usable without Ollama; show a connection note only when it is known to be
 * unsaved. Progress belongs on Duplicates, not here.
 */
export default function VersionJudgingSection() {
  const text = t.settings.versionJudging;
  const [settings, setSettings] = useState<VersionJudgingSettings | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const controller = useRef<AbortController | null>(null);
  const { connection } = useOllamaConnection();

  const load = useCallback(async () => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    setLoadError(null);
    try {
      setSettings(await getVersionJudgingSettings(abort.signal));
    } catch (failure) {
      if (isAborted(failure)) return;
      setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
    return () => controller.current?.abort();
  }, [load]);

  // Clearing saving rolls the switch back to the stored value on failure.
  const { saving, saveError, toggle } = useSwitchSave({
    settings,
    current: (saved) => saved.enabled,
    save: updateVersionJudgingSettings,
    saved: setSettings,
    saveFailed: text.saveFailed,
  });

  return (
    <PageSection
      title={text.heading}
      description={<PageLinkText sentence={text.description} page="connections" />}
    >
      {loadError !== null && settings === null && (
        <div>
          <ErrorState
            title={text.loadFailed(loadError)}
            retryLabel={t.common.retry}
            onRetry={() => void load()}
          />
        </div>
      )}
      {loadError === null && (
        <FormRow
          label={text.label}
          description={text.hint}
          htmlFor="version-judging-enabled"
        >
          {settings === null ? (
            // Skeleton is hidden from accessibility by default; announce loading separately.
            <Skeleton
              aria-hidden={false}
              role="status"
              aria-label={text.loading}
              className="h-5 w-9 rounded-full"
            />
          ) : (
            <Switch
              id="version-judging-enabled"
              checked={saving ?? settings.enabled}
              aria-disabled={saving !== null || undefined}
              onCheckedChange={() => void toggle()}
            />
          )}
        </FormRow>
      )}
      {loadError === null && connection.status === "unsaved" && (
        <p className="text-sm text-muted-foreground">
          <PageLinkText sentence={text.connectionNote} page="connections" />
        </p>
      )}
      <SaveStatus saving={saving !== null} savingText={text.saving} error={saveError} />
    </PageSection>
  );
}
