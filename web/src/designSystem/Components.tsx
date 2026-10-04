import { LayoutGrid, List, Plus, Tag as TagIcon, X } from "lucide-react";
import type { ReactNode } from "react";

import { t } from "../i18n";
import { Button } from "../ui/button";
import { Checkbox } from "../ui/checkbox";
import { Combobox } from "../ui/combobox";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "../ui/command";
import { Field, FieldDescription, FieldError, FieldLabel } from "../ui/field";
import { Input } from "../ui/input";
import { RadioGroup, RadioGroupItem } from "../ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";
import { Slider } from "../ui/slider";
import { Switch } from "../ui/switch";
import { Textarea } from "../ui/textarea";
import { Toggle } from "../ui/toggle";
import { ToggleGroup, ToggleGroupItem } from "../ui/toggle-group";

// 見本の部品の節（specs/038-design-system/ui-design.md「Review criteria」4）。部品ごとに
// variant を行、状態を列に並べる。hover と押下は index.css の data-demo-state で、
// キーボードのフォーカスは同じ輪を最初の子に描いて、触らずに見せる。

const states = ["normal", "hover", "focus", "pressed", "selected", "disabled"] as const;
type State = (typeof states)[number];

function demoState(state: State): string | undefined {
  if (state === "hover") return "hover";
  if (state === "focus") return "focus";
  if (state === "pressed") return "hover active";
  return undefined;
}

interface Row {
  variant: string;
  /** その状態の見本。持たない状態（ボタンの選択など）は null。 */
  render: (state: State) => ReactNode | null;
}

interface Demo {
  name: string;
  item: string;
  rows: Row[];
}

const buttonVariants = [
  "default",
  "secondary",
  "outline",
  "ghost",
  "destructive",
  "link",
] as const;

function demos(): Demo[] {
  const c = t.designSystem.component;
  const options = [1, 2, 3].map((n) => ({ value: String(n), label: c.option(n) }));
  return [
    {
      name: "Button",
      item: "button",
      rows: [
        ...buttonVariants.map((variant) => ({
          variant,
          render: (state: State) =>
            state === "selected" ? null : (
              <Button variant={variant} size="sm" disabled={state === "disabled"}>
                {c.action}
              </Button>
            ),
        })),
        ...(["sm", "default", "lg"] as const).map((size) => ({
          variant: `size ${size}`,
          render: (state: State) =>
            state === "selected" ? null : (
              <Button size={size} variant="outline" disabled={state === "disabled"}>
                <Plus aria-hidden="true" />
                {c.action}
              </Button>
            ),
        })),
        ...(["icon-sm", "icon"] as const).map((size) => ({
          variant: `size ${size}`,
          render: (state: State) =>
            state === "selected" ? null : (
              <Button
                size={size}
                variant="ghost"
                aria-label={c.iconAction}
                disabled={state === "disabled"}
              >
                <Plus aria-hidden="true" />
              </Button>
            ),
        })),
      ],
    },
    {
      name: "Input",
      item: "input",
      rows: [
        {
          variant: "default",
          render: (state) =>
            state === "selected" ? null : (
              <Input
                aria-label={c.label}
                placeholder={c.placeholder}
                disabled={state === "disabled"}
                className="w-3xs"
              />
            ),
        },
        {
          variant: "aria-invalid",
          render: (state) =>
            state === "selected" ? null : (
              <Input
                aria-label={c.label}
                aria-invalid
                defaultValue={c.text}
                disabled={state === "disabled"}
                className="w-3xs"
              />
            ),
        },
      ],
    },
    {
      name: "Textarea",
      item: "textarea",
      rows: [
        {
          variant: "default",
          render: (state) =>
            state === "selected" ? null : (
              <Textarea
                aria-label={c.label}
                placeholder={c.placeholder}
                disabled={state === "disabled"}
                className="w-3xs"
              />
            ),
        },
      ],
    },
    {
      name: "Select",
      item: "select",
      rows: [
        {
          variant: "default",
          render: (state) => (
            <Select
              defaultValue={state === "selected" ? "2" : undefined}
              disabled={state === "disabled"}
            >
              <SelectTrigger aria-label={c.label} size="sm" className="w-3xs">
                <SelectValue placeholder={c.choose} />
              </SelectTrigger>
              <SelectContent>
                {options.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ),
        },
      ],
    },
    {
      name: "RadioGroup",
      item: "radio-group",
      rows: [
        {
          variant: "item",
          render: (state) => (
            <RadioGroup
              aria-label={c.label}
              defaultValue={state === "selected" ? "1" : undefined}
              disabled={state === "disabled"}
            >
              <RadioGroupItem value="1" aria-label={c.option(1)} />
            </RadioGroup>
          ),
        },
      ],
    },
    {
      name: "Checkbox",
      item: "checkbox",
      rows: [
        {
          variant: "checked",
          render: (state) => (
            <Checkbox
              aria-label={c.label}
              defaultChecked={state === "selected"}
              disabled={state === "disabled"}
            />
          ),
        },
        {
          variant: "indeterminate",
          render: (state) =>
            state === "selected" ? (
              <Checkbox aria-label={c.label} defaultChecked="indeterminate" />
            ) : null,
        },
      ],
    },
    {
      name: "Switch",
      item: "switch",
      rows: [
        {
          variant: "default",
          render: (state) => (
            <Switch
              aria-label={c.setting}
              defaultChecked={state === "selected"}
              disabled={state === "disabled"}
            />
          ),
        },
      ],
    },
    {
      name: "Toggle",
      item: "toggle",
      rows: (["default", "outline"] as const).map((variant) => ({
        variant,
        render: (state: State) => (
          <Toggle
            variant={variant}
            size="sm"
            aria-label={c.filter}
            defaultPressed={state === "selected"}
            disabled={state === "disabled"}
          >
            <TagIcon aria-hidden="true" />
            {c.filter}
            <X aria-hidden="true" />
          </Toggle>
        ),
      })),
    },
    {
      name: "ToggleGroup",
      item: "toggle-group",
      rows: [
        {
          variant: "single outline",
          render: (state) => (
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              aria-label={c.view}
              defaultValue={state === "selected" ? "grid" : undefined}
              disabled={state === "disabled"}
            >
              <ToggleGroupItem value="grid" aria-label={c.grid}>
                <LayoutGrid aria-hidden="true" />
              </ToggleGroupItem>
              <ToggleGroupItem value="list" aria-label={c.list}>
                <List aria-hidden="true" />
              </ToggleGroupItem>
            </ToggleGroup>
          ),
        },
      ],
    },
    {
      name: "Slider",
      item: "slider",
      rows: [
        {
          variant: "steps",
          render: (state) =>
            state === "selected" ? null : (
              <Slider
                aria-label={c.zoom}
                defaultValue={[1]}
                min={0}
                max={3}
                step={1}
                disabled={state === "disabled"}
                className="h-8 w-zoom"
              />
            ),
        },
      ],
    },
    {
      name: "Combobox",
      item: "combobox",
      rows: [
        {
          variant: "default",
          render: (state) => (
            <ComboboxDemo
              options={options}
              selected={state === "selected"}
              disabled={state === "disabled"}
            />
          ),
        },
      ],
    },
  ];
}

function ComboboxDemo({
  options,
  selected,
  disabled,
}: {
  options: { value: string; label: string }[];
  selected: boolean;
  disabled: boolean;
}) {
  const c = t.designSystem.component;
  return (
    <Combobox
      aria-label={c.label}
      options={options}
      value={selected ? "2" : ""}
      onValueChange={() => undefined}
      placeholder={c.choose}
      searchPlaceholder={c.search}
      emptyText={c.empty}
      disabled={disabled}
      className="w-3xs"
    />
  );
}

function Code({ children }: { children: string }) {
  return <code className="font-mono text-xs text-foreground">{children}</code>;
}

function StateGrid({ demo }: { demo: Demo }) {
  const c = t.designSystem.component;
  return (
    <section data-component={demo.item} className="grid gap-2">
      <h3 className="flex items-baseline gap-2 text-sm font-semibold">
        {demo.name}
        <Code>{demo.item}</Code>
      </h3>
      <div className="overflow-x-auto rounded-lg border border-border bg-card">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="border-b border-border text-xs text-muted-foreground">
              <th scope="col" className="p-3 font-medium">
                {c.variant}
              </th>
              {states.map((state) => (
                <th key={state} scope="col" className="p-3 font-medium whitespace-nowrap">
                  {c.states[state]}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {demo.rows.map((row) => (
              <tr key={row.variant} className="border-b border-border last:border-b-0">
                <th
                  scope="row"
                  className="p-3 align-middle font-normal whitespace-nowrap"
                >
                  <Code>{row.variant}</Code>
                </th>
                {states.map((state) => {
                  const content = row.render(state);
                  return (
                    <td key={state} className="p-3 align-middle">
                      {content === null ? (
                        <span className="text-xs text-muted-foreground">{c.none}</span>
                      ) : (
                        <div
                          data-demo-state={demoState(state)}
                          className="flex min-h-10 items-center"
                        >
                          {content}
                        </div>
                      )}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

/** FieldDemo は Field に Label・Input・説明・誤りをまとめた 1 欄である。 */
function FieldDemo() {
  const c = t.designSystem.component;
  return (
    <section data-component="field" className="grid gap-2">
      <h3 className="flex items-baseline gap-2 text-sm font-semibold">
        {c.form}
        <Code>{"field"}</Code>
      </h3>
      <div className="grid gap-6 rounded-lg border border-border bg-card p-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="design-system-field">{c.label}</FieldLabel>
          <Input id="design-system-field" placeholder={c.placeholder} />
          <FieldDescription>{c.description}</FieldDescription>
        </Field>
        <Field data-invalid="true">
          <FieldLabel htmlFor="design-system-field-invalid">{c.label}</FieldLabel>
          <Input id="design-system-field-invalid" aria-invalid />
          <FieldError>{c.error}</FieldError>
        </Field>
        <Field orientation="horizontal">
          <Checkbox id="design-system-field-check" defaultChecked />
          <FieldLabel htmlFor="design-system-field-check">{c.setting}</FieldLabel>
        </Field>
        <Field orientation="horizontal">
          <Switch id="design-system-field-switch" />
          <FieldLabel htmlFor="design-system-field-switch">{c.setting}</FieldLabel>
        </Field>
      </div>
    </section>
  );
}

/** CommandDemo はポップオーバーに入れずに置いた Command（打って絞る一覧）である。 */
function CommandDemo() {
  const c = t.designSystem.component;
  return (
    <section data-component="command" className="grid gap-2">
      <h3 className="flex items-baseline gap-2 text-sm font-semibold">
        {"Command"}
        <Code>{"command"}</Code>
      </h3>
      <div className="max-w-sm rounded-lg border border-border bg-card p-3">
        <Command label={c.search} className="rounded-md border border-input">
          <CommandInput placeholder={c.search} />
          <CommandList label={c.search}>
            <CommandGroup heading={c.label}>
              {[1, 2, 3].map((n) => (
                <CommandItem key={n} value={c.option(n)} disabled={n === 3}>
                  {c.option(n)}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </div>
    </section>
  );
}

/**
 * Components は見本の部品の節である。操作と入力の部品を、通常・hover・キーボードの
 * フォーカス・押下・選択・無効の状態で並べ、メンテナーが確かめる
 * （specs/038-design-system/research.md R-1、R-2）。
 */
export default function Components() {
  return (
    <div className="grid gap-8">
      {demos().map((demo) => (
        <StateGrid key={demo.item} demo={demo} />
      ))}
      <FieldDemo />
      <CommandDemo />
    </div>
  );
}
