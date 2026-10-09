import type { ReactNode } from "react";

import { t } from "../i18n";
import { AdminTablePageExample } from "./blocks/admin-table-page-example";
import { CenteredFormExample } from "./blocks/centered-form-example";
import { ConfirmDialogExample } from "./blocks/confirm-dialog-example";
import { DetailPageExample } from "./blocks/detail-page-example";
import { FormDialogExample } from "./blocks/form-dialog-example";
import { GroupedListExample } from "./blocks/grouped-list-example";
import { ListPageExample } from "./blocks/list-page-example";
import { type ListState, ListStatesExample } from "./blocks/list-states-example";
import { SettingsPageExample } from "./blocks/settings-page-example";

// 見本の画面の型の節（specs/038-design-system/ui-design.md「Page patterns」と「Review
// criteria」6〜7）。骨格ごとに見本のブロックを枠に入れて並べ、一覧ページは状態ごとにも並べる。
// ダイアログは開くボタンを置き、押すと見本のダイアログが開く。

const listStates: ListState[] = [
  "loading",
  "empty",
  "error",
  "loadingMore",
  "loadMoreFailed",
];

function Code({ children }: { children: ReactNode }) {
  return (
    <code className="rounded-sm bg-muted px-1 font-mono text-xs text-muted-foreground">
      {children}
    </code>
  );
}

function Pattern({
  id,
  name,
  items,
  children,
}: {
  id: string;
  name: ReactNode;
  items: string[];
  children: ReactNode;
}) {
  return (
    <section data-pattern={id} className="grid gap-2">
      <h3 className="flex flex-wrap items-baseline gap-2 text-sm font-semibold">
        {name}
        {items.map((item) => (
          <Code key={item}>{item}</Code>
        ))}
      </h3>
      <div className="overflow-hidden rounded-lg border border-border bg-background">
        {children}
      </div>
    </section>
  );
}

/**
 * Patterns は見本の画面の型の節である。骨格・区画・状態を見本のデータで組んだブロックを
 * 並べ、メンテナーが 1440px と 390px で確かめる（specs/038-design-system/research.md R-1）。
 */
export default function Patterns() {
  const p = t.designSystem.pattern;
  return (
    <div className="grid gap-8">
      <p className="text-sm text-muted-foreground">{p.intro}</p>
      <Pattern
        id="list-page"
        name={p.names.listPage}
        items={[
          "list-page",
          "page-header",
          "toolbar",
          "card-grid",
          "selection-bar",
          "list-page-example",
        ]}
      >
        <ListPageExample />
      </Pattern>
      {listStates.map((state) => (
        <Pattern
          key={state}
          id={`list-state-${state}`}
          name={p.names.listState(p.states[state])}
          items={[
            state === "loading"
              ? "loading-state"
              : state === "empty"
                ? "empty-state"
                : state === "error"
                  ? "error-state"
                  : "load-more-row",
            "list-states-example",
          ]}
        >
          <ListStatesExample state={state} />
        </Pattern>
      ))}
      <Pattern
        id="grouped-list"
        name={p.names.groupedList}
        items={["list-page", "grouped-list", "grouped-list-example"]}
      >
        <GroupedListExample />
      </Pattern>
      <Pattern
        id="admin-table-page"
        name={p.names.adminTablePage}
        items={["admin-table-page", "data-table", "table", "admin-table-page-example"]}
      >
        <AdminTablePageExample />
      </Pattern>
      <Pattern
        id="settings-page"
        name={p.names.settingsPage}
        items={[
          "settings-page",
          "page-section",
          "form-row",
          "fact-list",
          "settings-page-example",
        ]}
      >
        <SettingsPageExample />
      </Pattern>
      <Pattern
        id="detail-page"
        name={p.names.detailPage}
        items={["detail-page", "page-section", "fact-list", "detail-page-example"]}
      >
        <DetailPageExample />
      </Pattern>
      <Pattern
        id="centered-form"
        name={p.names.centeredForm}
        items={["centered-form", "centered-form-example"]}
      >
        <CenteredFormExample />
      </Pattern>
      <Pattern
        id="dialogs"
        name={p.names.dialogs}
        items={[
          "form-dialog",
          "form-dialog-example",
          "confirm-dialog",
          "confirm-dialog-example",
        ]}
      >
        <div className="flex flex-wrap gap-2 p-4">
          <FormDialogExample />
          <ConfirmDialogExample />
        </div>
      </Pattern>
    </div>
  );
}
