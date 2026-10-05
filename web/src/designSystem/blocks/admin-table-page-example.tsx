import { Merge, MoreHorizontal, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { useState } from "react";

import { t } from "@/i18n";
import { AdminTablePage } from "@/ui/patterns/admin-table-page";
import { DataTable } from "@/ui/patterns/data-table";
import { PageHeader } from "@/ui/patterns/page-header";
import { SelectionBar } from "@/ui/patterns/selection-bar";
import { Toolbar } from "@/ui/patterns/toolbar";
import { Badge } from "@/ui/shadcn/badge";
import { Button } from "@/ui/shadcn/button";
import { Checkbox } from "@/ui/shadcn/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/ui/shadcn/dropdown-menu";
import { Input } from "@/ui/shadcn/input";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/ui/shadcn/table";
import { Tabs, TabsList, TabsTrigger } from "@/ui/shadcn/tabs";

import { ExampleTopBar } from "./list-page-example";

// 管理表ページの型の見本（registry:block admin-table-page-example）。トップバー（見本では
// 代わりの帯 ExampleTopBar）に検索、帯にタブ、表に
// チェックの列と行ごとの操作、選択中は一括の操作の選択バーを置く。写した画面は見本の
// タグを自分の行に差し替える（web/registry/rules/patterns.md の Admin table page）。

const videoCounts = [42, 18, 9, 27, 6, 13];

export function AdminTablePageExample() {
  const p = t.designSystem.pattern;
  const tags = Object.entries(p.tagNames).map(([id, name], index) => ({
    id,
    name,
    videos: videoCounts[index] ?? 0,
    tentative: index % 3 === 2,
  }));
  const [tab, setTab] = useState("all");
  const [selected, setSelected] = useState<ReadonlySet<string>>(() => new Set(["kyoto"]));
  const rows = tab === "tentative" ? tags.filter((tag) => tag.tentative) : tags;
  const allSelected = rows.length > 0 && rows.every((tag) => selected.has(tag.id));
  const someSelected = rows.some((tag) => selected.has(tag.id));
  const toggle = (id: string, on: boolean) => {
    setSelected((current) => {
      const next = new Set(current);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  };
  return (
    <div className="flex flex-col">
      <ExampleTopBar>
        <Toolbar
          placement="topBar"
          search={
            <div className="relative">
              <Search
                aria-hidden="true"
                className="pointer-events-none absolute top-2 left-2 size-4 text-muted-foreground"
              />
              <Input
                type="search"
                aria-label={p.searchTags}
                placeholder={p.searchTags}
                className="h-8 pl-8"
              />
            </div>
          }
        />
      </ExampleTopBar>
      <AdminTablePage
        header={
          <PageHeader
            title={p.tags}
            count={p.tagCount(tags.length)}
            actions={
              <Button size="sm">
                <Plus aria-hidden="true" />
                {p.newTag}
              </Button>
            }
          />
        }
        band={
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList>
              <TabsTrigger value="all">{p.allTags}</TabsTrigger>
              <TabsTrigger value="tentative">{p.tentativeTags}</TabsTrigger>
            </TabsList>
          </Tabs>
        }
        selectionBar={
          selected.size > 0 && (
            <SelectionBar
              count={p.selected(selected.size)}
              clearLabel={p.clearSelection}
              onClear={() => {
                setSelected(new Set());
              }}
            >
              <Button variant="ghost" size="sm">
                <Merge aria-hidden="true" />
                {p.merge}
              </Button>
              <Button variant="ghost" size="sm">
                <Trash2 aria-hidden="true" />
                {p.delete}
              </Button>
            </SelectionBar>
          )
        }
      >
        <DataTable label={p.tagTable}>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8">
                <Checkbox
                  aria-label={p.selectAll}
                  checked={allSelected ? true : someSelected ? "indeterminate" : false}
                  onCheckedChange={(checked) => {
                    for (const tag of rows) toggle(tag.id, checked === true);
                  }}
                />
              </TableHead>
              <TableHead>{p.name}</TableHead>
              <TableHead>{p.kind}</TableHead>
              <TableHead className="text-right">{p.videoCount}</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((tag) => (
              <TableRow
                key={tag.id}
                data-state={selected.has(tag.id) ? "selected" : undefined}
              >
                <TableCell>
                  <Checkbox
                    aria-label={p.selectTag(tag.name)}
                    checked={selected.has(tag.id)}
                    onCheckedChange={(checked) => {
                      toggle(tag.id, checked === true);
                    }}
                  />
                </TableCell>
                <TableCell className="font-medium">{tag.name}</TableCell>
                <TableCell>
                  <Badge variant={tag.tentative ? "warning" : "secondary"}>
                    {tag.tentative ? p.tentative : p.confirmed}
                  </Badge>
                </TableCell>
                <TableCell className="text-right tabular-nums">{tag.videos}</TableCell>
                <TableCell>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={p.tagActions(tag.name)}
                      >
                        <MoreHorizontal aria-hidden="true" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem>
                        <Pencil aria-hidden="true" />
                        {p.rename}
                      </DropdownMenuItem>
                      <DropdownMenuItem>
                        <Merge aria-hidden="true" />
                        {p.merge}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive">
                        <Trash2 aria-hidden="true" />
                        {p.delete}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </DataTable>
      </AdminTablePage>
    </div>
  );
}
