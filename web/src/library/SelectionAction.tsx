import { type ComponentProps, createContext, type ReactNode, useContext } from "react";

import type { UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/** True while a bar action waits on the server; every action under it is disabled meanwhile. */
export const SelectionBusyContext = createContext(false);

/** Keep action labels visible without hover; forward props for asChild popover/menu triggers. */
export function SelectionAction({
  icon,
  label,
  after,
  disabled,
  ...props
}: Omit<ComponentProps<typeof Button>, "children"> & {
  icon: ReactNode;
  label: UiText;
  after?: ReactNode;
}) {
  const busy = useContext(SelectionBusyContext);
  return (
    <Button
      variant="ghost"
      size="sm"
      aria-label={label}
      disabled={disabled === true || busy}
      {...props}
    >
      {icon}
      <span>{label}</span>
      {after}
    </Button>
  );
}

/** Disable an action with a reason that reads on hover and to screen readers through `reasonId`. */
export function disabledProps(reason: UiText | undefined, reasonId: string) {
  return {
    disabled: reason !== undefined,
    title: reason,
    "aria-describedby": reason === undefined ? undefined : reasonId,
  };
}

/** Screen-reader text for `disabledProps`; renders nothing while the action is enabled. */
export function DisabledReason({
  id,
  reason,
}: {
  id: string;
  reason: UiText | undefined;
}) {
  if (reason === undefined) return null;
  return (
    <span id={id} className="sr-only">
      {reason}
    </span>
  );
}

export function WithTooltip({ label, children }: { label: UiText; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
