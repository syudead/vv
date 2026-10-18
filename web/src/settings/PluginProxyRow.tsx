import { CircleCheck } from "lucide-react";
import { useRef, useState } from "react";

import { RequestFailed, updatePluginProxySettings } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { FormRow } from "../ui/patterns/form-row";
import { FieldError } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";

import { isPluginProxyURL } from "./pluginProxyURL";
import { SaveStatus } from "./SaveStatus";

const INPUT_ID = "network-plugin-proxy";
const DESCRIPTION_ID = "network-plugin-proxy-description";
const ERROR_ID = "network-plugin-proxy-error";

interface PluginProxySave {
  value: string;
  saving: boolean;
  saved: boolean;
  invalid: boolean;
  saveError: UiText | null;
  edit: (next: string) => void;
  commit: () => void;
}

/**
 * Serialize saves and submit the latest draft after an in-flight commit. Apply normalized
 * responses and errors only while the input still matches the submitted value, preserving
 * concurrent edits.
 */
function usePluginProxySave(initialURL: string): PluginProxySave {
  const [value, setValue] = useState(initialURL);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  // Async saves need the current input independently of render closures.
  const valueRef = useRef(initialURL);
  const savedURL = useRef(initialURL);
  const inFlight = useRef(false);
  const pending = useRef(false);

  const showFailure = (failure: unknown) => {
    if (failure instanceof RequestFailed && failure.status === 400) {
      setInvalid(true);
    } else {
      setSaveError(t.settings.network.proxy.saveFailed(errorText(failure)));
    }
  };

  const send = async (url: string) => {
    inFlight.current = true;
    setSaving(true);
    setSaved(false);
    setSaveError(null);
    const unchanged = () => valueRef.current.trim() === url;
    try {
      const result = await updatePluginProxySettings(url);
      savedURL.current = result.url;
      if (unchanged()) {
        valueRef.current = result.url;
        setValue(result.url);
        setSaved(true);
      }
    } catch (failure) {
      // Do not show errors for an obsolete submitted value after the input changes.
      if (unchanged()) showFailure(failure);
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
    if (pending.current) {
      pending.current = false;
      commit();
    }
  };

  function commit() {
    if (inFlight.current) {
      pending.current = true;
      return;
    }
    const url = valueRef.current.trim();
    if (url === savedURL.current) return;
    if (!isPluginProxyURL(url)) {
      setInvalid(true);
      return;
    }
    void send(url);
  }

  const edit = (next: string) => {
    valueRef.current = next;
    setValue(next);
    setInvalid(false);
    setSaved(false);
    setSaveError(null);
  };

  return { value, saving, saved, invalid, saveError, edit, commit };
}

/** Commit changed values on blur or Enter; there is no Save button. */
export function PluginProxyRow({ initialURL }: { initialURL: string }) {
  const network = t.settings.network;
  const text = network.proxy;
  const proxy = usePluginProxySave(initialURL);

  return (
    <>
      <FormRow
        label={text.label}
        description={text.description}
        descriptionId={DESCRIPTION_ID}
        htmlFor={INPUT_ID}
      >
        <div className="flex w-full flex-col gap-2 sm:w-sm">
          <Input
            id={INPUT_ID}
            type="url"
            autoComplete="off"
            spellCheck={false}
            placeholder={text.placeholder}
            value={proxy.value}
            onChange={(event) => proxy.edit(event.target.value)}
            onBlur={proxy.commit}
            onKeyDown={(event) => {
              if (event.key !== "Enter") return;
              event.preventDefault();
              proxy.commit();
            }}
            aria-invalid={proxy.invalid || undefined}
            aria-describedby={
              proxy.invalid ? `${DESCRIPTION_ID} ${ERROR_ID}` : DESCRIPTION_ID
            }
          />
          {proxy.invalid && <FieldError id={ERROR_ID}>{text.invalid}</FieldError>}
        </div>
      </FormRow>
      <SaveStatus
        saving={proxy.saving}
        savingText={network.saving}
        error={proxy.saveError}
      />
      {proxy.saved && (
        <p
          role="status"
          className="flex items-center gap-2 text-xs text-muted-foreground"
        >
          <CircleCheck aria-hidden="true" className="size-4" />
          {text.saved}
        </p>
      )}
    </>
  );
}
