import { Film, X } from "lucide-react";

import { t } from "../../i18n";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import { useFilePreview } from "./useFilePreview";

const imageOnly = ["image"] as const;

/** The file an image search runs on, at the search field's start, with the control back to the library. */
export function QueryChip({ file, onClear }: { file: File; onClear: () => void }) {
  const preview = useFilePreview(file, imageOnly);
  const label = t.library.imageSearch.clear;
  return (
    <Badge
      variant="secondary"
      data-slot="image-search-chip"
      title={file.name}
      className="max-w-full min-w-0 gap-1.5 pr-0.5 pl-0.5"
    >
      <span className="flex aspect-video h-5 shrink-0 items-center justify-center overflow-hidden rounded-sm bg-muted">
        {preview === null ? (
          <Film aria-hidden="true" className="text-muted-foreground" />
        ) : (
          <img src={preview} alt="" className="size-full object-cover" />
        )}
      </span>
      <span className="hidden min-w-0 truncate sm:inline">{file.name}</span>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            // Keep focus in the field so the chip's removal does not commit the typed text.
            onMouseDown={(event) => event.preventDefault()}
            onClick={onClear}
            className="size-5"
          >
            <X aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </Badge>
  );
}
