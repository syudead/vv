import { ArrowLeft } from "lucide-react";
import type { Ref } from "react";
import { Link } from "react-router";

import { t, type UiText } from "../i18n";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";

/** The header of a page below the tag list, with the back link to it. */
export function TagsSubpageHeader({
  title,
  description,
  titleRef,
}: {
  title: UiText;
  description: UiText;
  /** Focus target once nothing else on the page can take the focus. */
  titleRef?: Ref<HTMLHeadingElement>;
}) {
  return (
    <PageHeader
      title={title}
      description={description}
      titleRef={titleRef}
      leading={
        <Button asChild variant="ghost" size="sm">
          <Link to="/tags">
            <ArrowLeft aria-hidden="true" />
            {t.tags.title}
          </Link>
        </Button>
      }
    />
  );
}
