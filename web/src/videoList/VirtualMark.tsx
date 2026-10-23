import { SquareDashed } from "lucide-react";

import { t } from "../i18n";

/**
 * Mark in a virtual video's duration place; screen readers get only the sr-only text, which a card
 * link references through aria-describedby. Pass `announce={false}` when the row already announces it.
 */
export function VirtualMark({
  id,
  announce = true,
}: {
  id?: string;
  announce?: boolean;
}) {
  return (
    <>
      <SquareDashed aria-hidden="true" className="size-3 shrink-0" />
      <span aria-hidden="true">{t.list.card.virtual}</span>
      {announce && (
        <span id={id} className="sr-only">
          {t.list.card.virtualLabel}
        </span>
      )}
    </>
  );
}
