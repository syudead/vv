import type { ComponentProps } from "react";

import { Button } from "./shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "./shadcn/tooltip";

export function RowIconButton({
  label,
  ...props
}: Omit<ComponentProps<typeof Button>, "variant" | "size" | "aria-label"> & {
  label: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={label} {...props} />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
