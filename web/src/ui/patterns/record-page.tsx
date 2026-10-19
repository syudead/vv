import type { ReactNode } from "react";

import { PageColumn } from "./page-column";
import { PageHeader } from "./page-header";

// See web/registry/rules/patterns.md, Record page.

export interface RecordPageProps {
  /** The router link back to the list the record belongs to, as a `ghost` `sm` Button. */
  back: ReactNode;
  /** The record's name; it wraps in full instead of truncating. */
  title: ReactNode;
  /** Keep the title for screen readers only, while the record is loading or missing. */
  titleHidden?: boolean;
  /** The record's kind, one muted line under the title. */
  description?: ReactNode;
  /** At most one `outline` `sm` action that leaves for what the record is about. */
  actions?: ReactNode;
  /** One `Alert` for the record's condition, such as the full reason it failed. */
  notice?: ReactNode;
  /** `PageSection`s, or one state block. */
  children: ReactNode;
}

export function RecordPage({
  back,
  title,
  titleHidden = false,
  description,
  actions,
  notice,
  children,
}: RecordPageProps) {
  return (
    <PageColumn
      slot="record-page"
      header={
        <PageHeader
          title={title}
          titleHidden={titleHidden}
          wrapTitle
          description={description}
          leading={back}
          actions={actions}
        />
      }
      notice={notice}
    >
      {children}
    </PageColumn>
  );
}
