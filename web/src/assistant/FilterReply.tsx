import { Building2, Shapes, Tag as TagIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";

import type {
  AssistantCriteria,
  AssistantLabel,
  AssistantLabelKind,
} from "../api/assistant";
import { creditFace } from "../credits/creditFace";
import { t } from "../i18n";
import { conditionIcon } from "../library/conditionIcon";
import { videoConditionChips } from "../library/videoConditions";
import { cn } from "../lib/cn";
import { useSaveSmartFolder } from "../smartFolders/SaveSmartFolderButton";
import { cleanRule } from "../smartFolders/smartRule";
import { Button } from "../ui/shadcn/button";
import { toggleVariants } from "../ui/shadcn/toggle";
import { DEFAULT_SORT } from "../videoList/listCriteria";
import { criteriaPath, showsCriteria } from "./links";

interface Chip {
  key: string;
  icon: ReactNode;
  text: string;
  /** Names are capped like the library's chips; condition texts are short and built here. */
  capped: boolean;
}

function labelName(
  labels: readonly AssistantLabel[],
  kind: AssistantLabelKind,
  id: number,
): string | undefined {
  return labels.find((label) => label.kind === kind && label.id === id)?.name;
}

/** The library's Filtering by chips in its order, from the resolved criteria and their labels. */
export function criteriaChips({ rule, labels }: AssistantCriteria): Chip[] {
  const chips: Chip[] = [];
  const named = (key: string, icon: ReactNode, text: string | undefined) => {
    if (text !== undefined) chips.push({ key, icon, text, capped: true });
  };
  for (const id of rule.tags ?? []) {
    named(
      `tag:${String(id)}`,
      <TagIcon aria-hidden="true" />,
      labelName(labels, "tag", id),
    );
  }
  if (rule.person !== undefined) {
    named(
      "person",
      creditFace("person", { id: rule.person }, "token"),
      labelName(labels, "person", rule.person),
    );
  }
  if (rule.organization !== undefined) {
    named(
      "organization",
      <Building2 aria-hidden="true" />,
      labelName(labels, "organization", rule.organization),
    );
  }
  if (rule.tagKind !== undefined) {
    const name = labelName(labels, "tagKind", rule.tagKind);
    named(
      "tagKind",
      <Shapes aria-hidden="true" />,
      name === undefined ? undefined : t.library.activeTags.kindChip(name),
    );
  }
  for (const chip of videoConditionChips(rule)) {
    chips.push({
      key: `condition:${chip.kind}`,
      icon: conditionIcon(chip.kind, rule),
      text: chip.text,
      capped: false,
    });
  }
  return chips;
}

/** A filter applies at once: one line, the conditions as read-only chips, and the save link. */
export default function FilterReply({ criteria }: { criteria: AssistantCriteria }) {
  const location = useLocation();
  const save = useSaveSmartFolder();
  const chips = criteriaChips(criteria);
  const left = !showsCriteria(location.pathname, location.search, criteria);
  return (
    <div className="flex flex-col gap-2">
      <p className="flex flex-wrap items-center gap-x-2 text-sm">
        {t.assistant.filtered}
        {left && (
          <Button asChild variant="link" size="sm" className="h-auto p-0">
            <Link to={criteriaPath(criteria)}>{t.assistant.show}</Link>
          </Button>
        )}
      </p>
      {chips.length > 0 && (
        <ul aria-label={t.library.activeTags.list} className="flex flex-wrap gap-1">
          {chips.map((chip) => (
            <li key={chip.key}>
              <span
                data-state="on"
                title={chip.text}
                className={toggleVariants({ variant: "outline", size: "sm" })}
              >
                {chip.icon}
                <span
                  className={cn("min-w-0 truncate", chip.capped && "max-w-chip-label")}
                >
                  {chip.text}
                </span>
              </span>
            </li>
          ))}
        </ul>
      )}
      <div>
        <Button
          variant="link"
          size="sm"
          className="h-auto p-0"
          onClick={() =>
            save.open(cleanRule(criteria.rule), criteria.sort ?? DEFAULT_SORT)
          }
        >
          {t.assistant.saveSmartFolder}
        </Button>
      </div>
      {save.dialog}
    </div>
  );
}
