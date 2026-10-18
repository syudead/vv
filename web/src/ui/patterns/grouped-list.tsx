import type { ReactNode, Ref } from "react";

// This section owns group spacing and sticky headings; callers supply heading and row content. See web/registry/rules/patterns.md, Sections.

export interface GroupedListProps {
  label: string;
  children: ReactNode;
}

function GroupedListRoot({ label, children }: GroupedListProps) {
  return (
    <ul data-slot="grouped-list" aria-label={label} className="flex flex-col gap-6">
      {children}
    </ul>
  );
}

export interface GroupedListGroupProps {
  heading: ReactNode;
  detail?: ReactNode;
  children: ReactNode;
}

/**
 * Headings stick below the top bar until the next group pushes them out; hide the decorative
 * separator from assistive technology.
 */
function GroupedListGroup({ heading, detail, children }: GroupedListGroupProps) {
  return (
    <li data-slot="grouped-list-group" className="flex flex-col gap-2">
      <h2 className="sticky top-navbar z-10 flex flex-wrap items-baseline gap-x-1 bg-background py-2">
        <span className="text-sm font-semibold text-foreground">{heading}</span>
        {detail !== undefined && (
          <>
            {" "}
            <span aria-hidden="true" className="text-xs text-muted-foreground">
              ·
            </span>{" "}
            <span className="text-xs text-muted-foreground">{detail}</span>
          </>
        )}
      </h2>
      <ul className="flex flex-col">{children}</ul>
    </li>
  );
}

export interface GroupedListItemProps {
  children: ReactNode;
  ref?: Ref<HTMLLIElement>;
}

/** No horizontal padding so rows align with headings. */
function GroupedListItem({ children, ref }: GroupedListItemProps) {
  return (
    <li
      ref={ref}
      data-slot="grouped-list-item"
      className="flex min-w-0 items-center gap-3 py-2"
    >
      {children}
    </li>
  );
}

const GroupedList = Object.assign(GroupedListRoot, {
  Group: GroupedListGroup,
  Item: GroupedListItem,
});

export { GroupedList, GroupedListGroup, GroupedListItem };
