import { t } from "../i18n";
import { Label } from "../ui/shadcn/label";
import { Switch } from "../ui/shadcn/switch";

/** The word and the track are one control in a fixed column, so every folder switch of the page lines up. */
export function FolderSwitch({
  enabled,
  saving,
  label,
  onToggle,
}: {
  enabled: boolean;
  saving: boolean;
  label: string;
  onToggle: () => void;
}) {
  const text = t.settings.mediaFolders;
  return (
    <Label className="h-8 w-16 shrink-0 cursor-pointer justify-end text-xs font-normal text-muted-foreground">
      {enabled ? text.on : text.off}
      <Switch
        checked={enabled}
        aria-label={label}
        aria-disabled={saving || undefined}
        onCheckedChange={onToggle}
      />
    </Label>
  );
}
