import { FileSearch } from "lucide-react";
import { type FormEvent, useId, useRef, useState } from "react";

import type { WorkInfoPlugin } from "../../api/workInfoPlugins";
import { t, type UiText } from "../../i18n";
import {
  approvedWorkPlugins,
  canChooseFindBy,
  pluginLabel,
} from "../../player/workInfoFetch/fetchRules";
import { useApprovedPlugins } from "../../player/workInfoFetch/useApprovedPlugins";
import { Field, FieldLabel } from "../../ui/shadcn/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/shadcn/select";
import { CreateDialogFrame } from "../NameDialogParts";
import {
  DisabledReason,
  disabledProps,
  SelectionAction,
  WithTooltip,
} from "../SelectionAction";
import FillReviewDialog from "./FillReviewDialog";
import type { FillTargets } from "./useFillReview";

type Targets = FillTargets;

// A fill searches, then fetches the matching candidate.
function fillPlugins(plugins: readonly WorkInfoPlugin[]): WorkInfoPlugin[] {
  return approvedWorkPlugins(plugins).filter(canChooseFindBy);
}

/**
 * Pick a plugin, review the works its search finds for each selected video, then queue fills of the kept ones
 * (docs/design-docs/link-fetch.md). The selection stays and the cards update as each fill lands.
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
  const [pluginId, setPluginId] = useState<number | null>(null);
  if (pluginId !== null) {
    return <FillReviewDialog pluginId={pluginId} targets={targets} onClose={onClose} />;
  }
  return <PluginDialog onChosen={setPluginId} onClose={onClose} />;
}

function PluginDialog({
  onChosen,
  onClose,
}: {
  onChosen: (pluginId: number) => void;
  onClose: () => void;
}) {
  const words = t.library.selection.fillDialog;
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const { plugins, loaded } = useApprovedPlugins(fillPlugins);
  const [chosen, setChosen] = useState<number | undefined>(undefined);
  const plugin = plugins.find((item) => item.id === chosen) ?? plugins[0];

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (plugin !== undefined) onChosen(plugin.id);
  }

  return (
    <CreateDialogFrame
      words={words}
      description={words.description}
      pending={false}
      failure={null}
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
            disabled={plugin === undefined}
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
