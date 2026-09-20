import { NavLink } from "react-router";

import Icon from "./icons";
import type { InertNavItem, LiveNavItem, NavItem as NavigationItem } from "./navigation";

const focus =
  "outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus";

export default function NavItem({
  item,
  collapsed,
}: {
  item: NavigationItem;
  collapsed: boolean;
}) {
  return item.kind === "live" ? (
    <LiveRow item={item} collapsed={collapsed} />
  ) : (
    <InertRow item={item} collapsed={collapsed} />
  );
}

function rowClass(collapsed: boolean): string {
  return (
    "relative flex min-h-[var(--size-tap)] items-center rounded-control text-sm " +
    (collapsed ? "justify-center px-1 " : "gap-2.5 px-3 ")
  );
}

function Label({ label, collapsed }: { label: string; collapsed: boolean }) {
  return (
    <span className={collapsed ? "sr-only" : "min-w-0 flex-1 truncate"}>{label}</span>
  );
}

function LiveRow({ item, collapsed }: { item: LiveNavItem; collapsed: boolean }) {
  return (
    <NavLink
      to={item.to}
      end
      data-nav-id={item.id}
      title={collapsed ? item.label : undefined}
      className={({ isActive }) =>
        `${rowClass(collapsed)} ${focus} transition-colors motion-reduce:transition-none ` +
        (isActive
          ? "bg-accent-surface text-accent before:absolute before:inset-y-2 before:left-0 before:w-0.5 before:rounded-full before:bg-accent hover:bg-accent-surface/80 active:bg-surface-sunken"
          : "text-body hover:bg-body/10 active:bg-surface-sunken active:text-accent")
      }
    >
      <Icon name={item.icon} className="h-5 w-5 shrink-0" />
      <Label label={item.label} collapsed={collapsed} />
    </NavLink>
  );
}

function InertRow({ item, collapsed }: { item: InertNavItem; collapsed: boolean }) {
  return (
    <span
      data-nav-id={item.id}
      title={collapsed ? `${item.label}（未実装）` : undefined}
      className={`${rowClass(collapsed)} text-muted`}
    >
      <Icon name={item.icon} className="h-5 w-5 shrink-0" />
      <Label label={item.label} collapsed={collapsed} />
      <span className="sr-only">（未実装）</span>
    </span>
  );
}
