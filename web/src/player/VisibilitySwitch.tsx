import { AlertCircle, Globe, LoaderCircle, Lock } from "lucide-react";
import { useState } from "react";

import { updateVideoVisibility } from "../api/visibility";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";

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

  const Icon = sending ? LoaderCircle : isPublic ? Globe : Lock;

  return (
    <div className="flex flex-col items-start gap-1 sm:flex-row sm:items-center sm:gap-3">
      <Button
        variant="secondary"
        size="sm"
        role="switch"
        aria-checked={isPublic}
        aria-label={t.player.visibility.label}
        aria-disabled={sending || undefined}
        onClick={toggle}
        className={cn(
          "px-visibility-x text-xs has-[>svg]:px-visibility-x aria-disabled:cursor-default aria-disabled:opacity-50",
          isPublic &&
            "bg-primary-soft text-primary hover:bg-primary-soft hover:text-foreground",
        )}
      >
        <Icon
          aria-hidden="true"
          className={cn(sending && "animate-spin motion-reduce:animate-none")}
        />
        {isPublic ? t.player.visibility.public : t.player.visibility.private}
      </Button>
      {failure !== null && (
        <p role="alert" className="flex items-center gap-1.5 text-sm text-destructive">
          <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
          {failure}
        </p>
      )}
    </div>
  );
}
