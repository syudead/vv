import { FileSearch } from "lucide-react";
import { type FormEvent, useId, useRef, useState } from "react";

import { startWorkInfoFills, type WorkInfoPlugin } from "../api/workInfoPlugins";
import { errorText, t, type UiText } from "../i18n";
import {
  approvedWorkPlugins,
  canChooseFindBy,
  pluginLabel,
} from "../player/workInfoFetch/fetchRules";
import { useApprovedPlugins } from "../player/workInfoFetch/useApprovedPlugins";
import { useToast } from "../ui/Toast";
import { Field, FieldLabel } from "../ui/shadcn/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/shadcn/select";
import { CreateDialogFrame } from "./NameDialogParts";
import {
  DisabledReason,
  disabledProps,
  SelectionAction,
  WithTooltip,
} from "./SelectionAction";

interface Targets {
  videoIds: readonly number[];
  smartFolderIds: readonly number[];
}

// A fill searches, then fetches the matching candidate.
function fillPlugins(plugins: readonly WorkInfoPlugin[]): WorkInfoPlugin[] {
  return approvedWorkPlugins(plugins).filter(canChooseFindBy);
}

/**
 * Queue fills of the selection's empty fields from one plugin's search
 * (docs/design-docs/link-fetch.md). The server answers before any plugin runs, so the
 * selection stays and the cards update as each fill lands.
 */
export default function FillInfoAction({
  videoIds,
  smartFolderIds,
  disabledReason,
}: Targets & { disabledReason: UiText | undefined }) {
  const reasonId = useId();
  // Freeze the targets at opening; the modal dialog keeps the selection from changing meanwhile.
  const [targets, setTargets] = useState<Targets | null>(null);
  return (
    <>
      <WithTooltip label={disabledReason ?? t.library.selection.fillInfo}>
        <SelectionAction
          icon={<FileSearch aria-hidden="true" />}
          label={t.library.selection.fillInfo}
          {...disabledProps(disabledReason, reasonId)}
          onClick={() => setTargets({ videoIds, smartFolderIds })}
        />
      </WithTooltip>
      <DisabledReason id={reasonId} reason={disabledReason} />
      {targets !== null && (
        <FillInfoDialog targets={targets} onClose={() => setTargets(null)} />
      )}
    </>
  );
}

function FillInfoDialog({ targets, onClose }: { targets: Targets; onClose: () => void }) {
  const words = t.library.selection.fillDialog;
  const toast = useToast();
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const { plugins, loaded } = useApprovedPlugins(fillPlugins);
  const [chosen, setChosen] = useState<number | undefined>(undefined);
  const plugin = plugins.find((item) => item.id === chosen) ?? plugins[0];
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || plugin === undefined) return;
    setPending(true);
    setFailure(null);
    startWorkInfoFills(plugin.id, targets.videoIds, targets.smartFolderIds).then(
      (queued) => {
        toast(t.library.selection.fillsQueued(queued.queued));
        onClose();
      },
      (error: unknown) => {
        setPending(false);
        setFailure(words.failed(errorText(error)));
      },
    );
  }

  return (
    <CreateDialogFrame
      words={words}
      description={words.description}
      pending={pending}
      failure={failure}
      initialFocus={trigger}
      submitDisabled={plugin === undefined}
      onSubmit={submit}
      onClose={onClose}
    >
      {loaded && plugin === undefined ? (
        <p className="text-sm text-muted-foreground">{words.noPlugin}</p>
      ) : (
        <Field>
          <FieldLabel htmlFor={id}>{words.plugin}</FieldLabel>
          <Select
            value={plugin === undefined ? "" : String(plugin.id)}
            onValueChange={(value) => setChosen(Number(value))}
            disabled={pending || plugin === undefined}
          >
            <SelectTrigger ref={trigger} id={id} size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {plugins.map((item) => (
                <SelectItem key={item.id} value={String(item.id)}>
                  {pluginLabel(item)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}
    </CreateDialogFrame>
  );
}
