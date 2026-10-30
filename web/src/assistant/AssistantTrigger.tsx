import { BotMessageSquare } from "lucide-react";

import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { assistantPanelId } from "./AssistantPanel";
import { useAssistant } from "./AssistantProvider";

/** The panel's toggle in the top bar and the video header band; guests never see it. */
export default function AssistantTrigger() {
  const store = useAssistant();
  const owner = useAudience() === "owner";
  if (store === null || !owner) return null;
  const working = store.working && !store.open;
  const awaiting = store.awaiting && !store.open;
  const label = working
    ? t.assistant.working
    : awaiting
      ? t.assistant.awaiting
      : t.assistant.trigger;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          data-assistant-trigger=""
          variant="ghost"
          size="icon-sm"
          className="relative"
          aria-label={label}
          aria-expanded={store.open}
          aria-controls={store.open ? assistantPanelId : undefined}
          onClick={() => store.toggle(true)}
        >
          <BotMessageSquare aria-hidden="true" />
          {(working || awaiting) && (
            <span
              aria-hidden="true"
              className="absolute top-1 right-1 size-2 rounded-full bg-primary"
            />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
