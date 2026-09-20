import Icon from "./icons";
import NavItem from "./NavItem";
import { itemsIn } from "./navigation";
import SidebarSection from "./SidebarSection";

export default function Sidebar({
  collapsed,
  onToggle,
}: {
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <aside
      className={
        "fixed inset-y-0 left-0 z-30 hidden h-dvh w-[var(--sidebar-current)] " +
        "border-r border-border bg-surface-raised transition-[width] motion-reduce:transition-none sm:block"
      }
    >
      <div className="flex h-full flex-col px-2 py-3">
        <div className="flex h-9 items-center justify-end">
          <button
            type="button"
            onClick={onToggle}
            aria-label={collapsed ? "サイドバーを展開" : "サイドバーを折りたたむ"}
            title={collapsed ? "サイドバーを展開" : "サイドバーを折りたたむ"}
            className="flex h-8 w-8 items-center justify-center rounded-control border border-transparent bg-surface-sunken text-muted outline-none transition-colors hover:border-border hover:bg-body/10 hover:text-body active:bg-surface-sunken active:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus motion-reduce:transition-none"
          >
            <Icon name={collapsed ? "arrowRight" : "arrowLeft"} className="h-4 w-4" />
          </button>
        </div>

        <nav className="mt-2" aria-label="メインナビゲーション">
          <SidebarSection>
            {itemsIn("library").map((item) => (
              <li key={item.id}>
                <NavItem item={item} collapsed={collapsed} />
              </li>
            ))}
          </SidebarSection>
        </nav>
      </div>
    </aside>
  );
}
