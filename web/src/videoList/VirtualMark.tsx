import { SquareDashed } from "lucide-react";

import { t } from "../i18n";

/**
 * Noninteractive mark in a virtual video's duration place. The visible word is hidden from screen
 * readers; a card link references the sr-only text through aria-describedby. A copy that repeats
 * a mark already announced elsewhere in the same row passes `announce={false}`.
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
