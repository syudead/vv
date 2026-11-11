import type { ReactNode } from "react";

import { type BackgroundWork, setBackgroundPause } from "../api/background";
import { t } from "../i18n";
import type { BackgroundSource } from "../shell/BackgroundProvider";
import { FormRow } from "../ui/patterns/form-row";
import { Switch } from "../ui/shadcn/switch";

import { SaveStatus } from "./SaveStatus";
import { useSwitchSave } from "./useSwitchSave";

/**
 * The switch moves at once and stays aria-disabled until the answer; a failure returns it to the
 * last status read and stays under the row until the next press.
 */
export default function BackgroundPauseSwitch({
  source,
  status,
  id,
  description,
}: {
  source: BackgroundSource;
  status: BackgroundWork;
  id: string;
  description?: ReactNode;
}) {
  const text = t.shell.background;
  const { saving, saveError, toggle } = useSwitchSave({
    settings: status,
    current: (current) => current.paused,
    save: setBackgroundPause,
    saved: source.saved,
    // The press inverts the status of this render, so its direction is known here.
    saveFailed: status.paused ? text.resumeFailed : text.pauseFailed,
    onStart: source.beginSave,
    onSettled: source.endSave,
  });

  return (
    <>
      <FormRow label={text.switchLabel} description={description} htmlFor={id}>
        <Switch
          id={id}
          checked={saving ?? status.paused}
          aria-disabled={saving !== null || undefined}
          onCheckedChange={() => void toggle()}
        />
      </FormRow>
      <SaveStatus
        saving={saving !== null}
        savingText={saving === true ? text.pausing : text.resuming}
        error={saveError}
      />
    </>
  );
}
