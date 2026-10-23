import { AlertCircle } from "lucide-react";
import { useState } from "react";

import { updateVideoVisibility } from "../api/visibility";
import { errorText, t, type UiText } from "../i18n";
import { Label } from "../ui/shadcn/label";
import { Switch } from "../ui/shadcn/switch";

/** Apply visibility only after success; aria-disabled preserves keyboard focus while blocking duplicate submissions. Callers key by video ID to reset errors. */
export default function VisibilitySwitch({
  videoId,
  isPublic,
  onChanged,
}: {
  videoId: number;
  isPublic: boolean;
  /** Refresh video data after success because its updated timestamp changes. */
  onChanged?: () => void;
}) {
  const [sending, setSending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  function toggle() {
    if (sending) return;
    setFailure(null);
    setSending(true);
    updateVideoVisibility([videoId], !isPublic)
      .then(() => onChanged?.())
      .catch((error: unknown) => setFailure(t.player.visibility.failed(errorText(error))))
      .finally(() => setSending(false));
  }

  return (
    <div className="flex flex-col items-end gap-1 sm:flex-row sm:items-center sm:gap-3">
      {/* The word and the track are one control: pressing either toggles. */}
      <Label className="h-8 cursor-pointer px-1 text-xs font-normal text-foreground">
        {isPublic ? t.player.visibility.public : t.player.visibility.private}
        <Switch
          checked={isPublic}
          aria-label={t.player.visibility.label}
          aria-disabled={sending || undefined}
          aria-busy={sending || undefined}
          onCheckedChange={toggle}
        />
      </Label>
      {failure !== null && (
        <p role="alert" className="flex items-center gap-1.5 text-sm text-destructive">
          <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
          {failure}
        </p>
      )}
    </div>
  );
}
