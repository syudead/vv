import type { ReactNode } from "react";

import { updateAutoImportSettings } from "../api/client";
import { t } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Switch } from "../ui/shadcn/switch";

import { SaveStatus } from "./SaveStatus";
import type { AutoImportSource } from "./useAutoImportSettings";
import { useSwitchSave } from "./useSwitchSave";

/** Saving immediately does not start progress notifications; the source need not follow watcher state. */
export default function AutoImportSwitch({
  source,
  description,
}: {
  source: AutoImportSource;
  description?: ReactNode;
}) {
  const text = t.settings.autoImport;
  const { settings, loadError } = source;

  const { saving, saveError, toggle } = useSwitchSave({
    settings,
    current: (saved) => saved.enabled,
    save: updateAutoImportSettings,
    saved: source.saved,
    saveFailed: text.saveFailed,
    onStart: source.beginSave,
    onSettled: source.endSave,
  });

  const checked = saving ?? settings?.enabled ?? false;

  return (
    <>
      {loadError !== null && settings === null && (
        <div>
          <ErrorState
            title={text.loadFailed(loadError)}
            retryLabel={t.common.retry}
            onRetry={() => void source.load()}
          />
        </div>
      )}
      {loadError === null && settings === null && (
        <FormRow label={text.label} htmlFor="auto-import-enabled">
          <Skeleton
            role="status"
            aria-label={text.loading}
            className="h-5 w-9 rounded-full"
          />
        </FormRow>
      )}
      {settings !== null && (
        <FormRow
          label={text.label}
          description={description}
          htmlFor="auto-import-enabled"
        >
          <Switch
            id="auto-import-enabled"
            checked={checked}
            aria-disabled={saving !== null || undefined}
            onCheckedChange={() => void toggle()}
          />
        </FormRow>
      )}
      <SaveStatus saving={saving !== null} savingText={text.saving} error={saveError} />
    </>
  );
}
