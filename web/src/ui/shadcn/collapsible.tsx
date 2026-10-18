import { Collapsible as CollapsiblePrimitive } from "radix-ui";
import type { ComponentProps } from "react";

// Adapted from shadcn/ui Radix Collapsible, taken as upstream.

function Collapsible({ ...props }: ComponentProps<typeof CollapsiblePrimitive.Root>) {
  return <CollapsiblePrimitive.Root data-slot="collapsible" {...props} />;
}

function CollapsibleTrigger({
  ...props
}: ComponentProps<typeof CollapsiblePrimitive.CollapsibleTrigger>) {
  return (
    <CollapsiblePrimitive.CollapsibleTrigger data-slot="collapsible-trigger" {...props} />
  );
}

function CollapsibleContent({
  ...props
}: ComponentProps<typeof CollapsiblePrimitive.CollapsibleContent>) {
  return (
    <CollapsiblePrimitive.CollapsibleContent data-slot="collapsible-content" {...props} />
  );
}

export { Collapsible, CollapsibleContent, CollapsibleTrigger };
