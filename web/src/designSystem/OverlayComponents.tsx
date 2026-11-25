import {
  Activity,
  AlertTriangle,
  CircleAlert,
  Folder,
  Globe,
  Info,
  Library,
  Lock,
  Settings,
  Tags,
  Trash2,
  VideoOff,
} from "lucide-react";
import type { MouseEvent, ReactNode } from "react";
import { toast } from "sonner";

import { t } from "../i18n";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "../ui/shadcn/alert-dialog";
import { Badge } from "../ui/shadcn/badge";
import BrandHomeLink from "../ui/BrandHomeLink";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "../ui/shadcn/breadcrumb";
import { Button } from "../ui/shadcn/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "../ui/shadcn/dialog";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "../ui/shadcn/empty";
import FavoriteToggle from "../ui/FavoriteToggle";
import { FilterDropdownButton } from "../ui/FilterDropdownButton";
import { Kbd, KbdGroup } from "../ui/shadcn/kbd";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "../ui/shadcn/popover";
import { Progress } from "../ui/shadcn/progress";
import { Separator } from "../ui/shadcn/separator";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuCount,
  SidebarMenuItem,
  SidebarProvider,
} from "../ui/shadcn/sidebar";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Toaster } from "../ui/shadcn/sonner";
import { Spinner } from "../ui/shadcn/spinner";
import { Tabs, TabsList, TabsTrigger } from "../ui/shadcn/tabs";
import { FloatingNotice } from "../ui/FloatingNotice";
import { KindMark } from "../ui/KindMark";
import { KindToggleGroup } from "../ui/KindToggleGroup";
import { RateRing } from "../ui/RateRing";
import { RecordCard, type RecordCardProps } from "../ui/RecordCard";
import RemoteMark from "../ui/RemoteMark";
import { TallyStrip } from "../ui/TallyStrip";
import TentativeMark from "../ui/TentativeMark";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailMark,
  VideoThumbnailNotice,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";

type Preview = "hover" | "focus" | "active";

/** Pressed demos also show hover. */
const demoState: Record<Preview, string> = {
  hover: "hover",
  focus: "focus",
  active: "hover active",
};

/** Search syntax tokens are not translated. */
const searchKeys = ["tag:", "-", '"…"'] as const;

function Code({ children }: { children: string }) {
  return <code className="font-mono text-xs text-foreground">{children}</code>;
}

function Item({
  name,
  children,
  wide = false,
}: {
  name: string;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <div
      className={`grid content-start gap-3 rounded-lg border border-border bg-background p-4 ${wide ? "lg:col-span-2" : ""}`}
    >
      <Code>{name}</Code>
      {children}
    </div>
  );
}

/** Use display: contents to preserve direct-child layout; draw focus on the first child. */
function Demo({ state, children }: { state?: Preview; children: ReactNode }) {
  if (state === undefined) return <>{children}</>;
  return (
    <span className="contents" data-demo-state={demoState[state]}>
      {children}
    </span>
  );
}

function State({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid justify-items-start gap-1.5">
      {children}
      <span className="text-xs text-muted-foreground">{label}</span>
    </div>
  );
}

function stateList(): { label: string; preview?: Preview }[] {
  const s = t.designSystem.overlay.states;
  return [
    { label: s.normal },
    { label: s.hover, preview: "hover" },
    { label: s.focus, preview: "focus" },
    { label: s.pressed, preview: "active" },
  ];
}

function Overlays() {
  const o = t.designSystem.overlay;
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Item name="Dialog">
        <Dialog>
          <DialogTrigger asChild>
            <Button variant="secondary" size="sm" className="justify-self-start">
              {o.openDialog}
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{o.dialogTitle}</DialogTitle>
              <DialogDescription>{o.dialogDescription}</DialogDescription>
            </DialogHeader>
            <p>{o.dialogBody}</p>
            <DialogFooter>
              <Button variant="outline" size="sm">
                {t.common.cancel}
              </Button>
              <Button size="sm">{o.merge}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </Item>

      <Item name="AlertDialog">
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="secondary" size="sm" className="justify-self-start">
              <Trash2 />
              {o.openAlertDialog}
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{o.alertTitle}</AlertDialogTitle>
              <AlertDialogDescription>{o.alertDescription}</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t.common.cancel}</AlertDialogCancel>
              <AlertDialogAction variant="destructive">{o.delete}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </Item>

      <Item name="Popover">
        <div className="min-h-card-0">
          <Popover open>
            <PopoverTrigger asChild>
              <Button variant="secondary" size="sm">
                {o.popoverTrigger}
              </Button>
            </PopoverTrigger>
            <PopoverContent
              align="start"
              onOpenAutoFocus={(event) => event.preventDefault()}
            >
              <PopoverHeader>
                <PopoverTitle>{o.popoverTitle}</PopoverTitle>
                <PopoverDescription>{o.popoverDescription}</PopoverDescription>
              </PopoverHeader>
              <Progress value={60} aria-label={o.popoverTitle} />
            </PopoverContent>
          </Popover>
        </div>
      </Item>

      <Item name="DropdownMenu">
        <div className="min-h-card-0">
          <DropdownMenu open modal={false}>
            <DropdownMenuTrigger asChild>
              <Button variant="secondary" size="sm">
                {o.menuTrigger}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent onCloseAutoFocus={(event) => event.preventDefault()}>
              <DropdownMenuLabel>{o.menuLabel}</DropdownMenuLabel>
              <DropdownMenuItem>
                <Globe />
                {o.makePublic}
                <span className="ml-auto text-xs text-muted-foreground">
                  {o.states.normal}
                </span>
              </DropdownMenuItem>
              <Demo state="focus">
                <DropdownMenuItem>
                  <Lock />
                  {o.makePrivate}
                  <span className="ml-auto text-xs text-muted-foreground">
                    {o.states.hover}
                  </span>
                </DropdownMenuItem>
              </Demo>
              <DropdownMenuCheckboxItem checked>{o.favorite}</DropdownMenuCheckboxItem>
              <DropdownMenuItem disabled>{o.states.disabled}</DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive">
                <Trash2 />
                {o.removeTag}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </Item>

      <Item name="Tooltip">
        <div className="pt-8">
          <Tooltip open>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="sm">
                {o.states.open}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">{o.tooltip}</TooltipContent>
          </Tooltip>
        </div>
      </Item>

      <Item name="Tabs">
        <div className="grid gap-4">
          <Tabs defaultValue="all">
            <TabsList>
              <TabsTrigger value="all">{o.tabs.all}</TabsTrigger>
              <Demo state="hover">
                <TabsTrigger value="tentative">{o.tabs.tentative}</TabsTrigger>
              </Demo>
              <TabsTrigger value="rejected" disabled>
                {o.tabs.rejected}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <Tabs defaultValue="all">
            <TabsList variant="line">
              <TabsTrigger value="all">{o.tabs.all}</TabsTrigger>
              <Demo state="focus">
                <TabsTrigger value="tentative">{o.tabs.tentative}</TabsTrigger>
              </Demo>
              <TabsTrigger value="rejected" disabled>
                {o.tabs.rejected}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <p className="text-xs text-muted-foreground">
            {`${o.states.selected} · ${o.states.hover} · ${o.states.disabled} / ${o.states.selected} · ${o.states.focus} · ${o.states.disabled}`}
          </p>
        </div>
      </Item>

      <Item name="Sonner">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="secondary" size="sm" onClick={() => toast(o.toastMessage)}>
            {o.toast}
          </Button>
        </div>
        <Toaster position="bottom-right" />
      </Item>
    </div>
  );
}

function Feedback() {
  const o = t.designSystem.overlay;
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Item name="Badge">
        <div className="flex flex-wrap items-center gap-2">
          <Badge>{o.badges.count}</Badge>
          <Badge variant="secondary">{o.badges.tag}</Badge>
          <Badge variant="soft">{o.badges.selected}</Badge>
          <Badge variant="outline">{o.badges.tag}</Badge>
          <Badge variant="warning">
            <AlertTriangle />
            {o.badges.warning}
          </Badge>
          <Badge variant="destructive">
            <CircleAlert />
            {o.badges.error}
          </Badge>
          <Badge variant="success">{o.badges.done}</Badge>
        </div>
        <div className="flex flex-wrap items-end gap-4">
          {stateList().map((state) => (
            <State key={state.label} label={state.label}>
              <Demo state={state.preview}>
                <Badge asChild variant="secondary">
                  <a href="#components">
                    {o.badges.tag}
                    <TentativeMark />
                  </a>
                </Badge>
              </Demo>
            </State>
          ))}
        </div>
      </Item>

      <Item name="Alert">
        <Alert variant="destructive">
          <CircleAlert />
          <AlertTitle>{o.alerts.errorTitle}</AlertTitle>
          <AlertDescription>{o.alerts.errorDescription}</AlertDescription>
        </Alert>
        <Alert variant="warning">
          <AlertTriangle />
          <AlertTitle>{o.alerts.warningTitle}</AlertTitle>
          <AlertDescription>{o.alerts.warningDescription}</AlertDescription>
        </Alert>
        <Alert>
          <Info />
          <AlertTitle>{o.alerts.infoTitle}</AlertTitle>
          <AlertDescription>{o.alerts.infoDescription}</AlertDescription>
        </Alert>
      </Item>

      <Item name="Empty">
        <Empty className="border border-border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <VideoOff />
            </EmptyMedia>
            <EmptyTitle>{o.empty.title}</EmptyTitle>
            <EmptyDescription>{o.empty.description}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      </Item>

      <Item name="Skeleton · Spinner · Progress">
        <div className="flex items-center gap-3">
          <Skeleton className="aspect-video w-list-thumb" />
          <div className="grid flex-1 gap-2">
            <Skeleton className="h-4 w-3/5" />
            <Skeleton className="h-3 w-1/3" />
          </div>
        </div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner />
          {o.loading}
        </div>
        <Progress value={40} aria-label={o.progress} />
        <Progress value={null} aria-label={o.progress} />
      </Item>

      <Item name="Separator · Kbd · Breadcrumb">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink href="#components">{o.breadcrumb.root}</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <Demo state="hover">
                <BreadcrumbLink href="#components">{o.breadcrumb.parent}</BreadcrumbLink>
              </Demo>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{o.breadcrumb.current}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <Separator />
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          {o.kbd}
          <KbdGroup>
            {searchKeys.map((key) => (
              <Kbd key={key}>{key}</Kbd>
            ))}
          </KbdGroup>
        </div>
      </Item>

      <Item name="Sidebar">
        <SidebarProvider className="min-h-0">
          <div className="flex w-full gap-6">
            <div className="w-sidebar rounded-md bg-navbar p-2">
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton isActive>
                    <Library />
                    <span>{`${o.sidebar.library} · ${o.states.selected}`}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                {stateList()
                  .slice(1)
                  .map((state) => (
                    <SidebarMenuItem key={state.label}>
                      <Demo state={state.preview}>
                        <SidebarMenuButton>
                          <Folder />
                          <span>{`${o.sidebar.folders} · ${state.label}`}</span>
                        </SidebarMenuButton>
                      </Demo>
                    </SidebarMenuItem>
                  ))}
                <SidebarMenuItem>
                  <SidebarMenuButton disabled>
                    <Tags />
                    <span>{`${o.sidebar.tags} · ${o.states.disabled}`}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </div>
            <div className="group w-12 rounded-md bg-navbar p-2" data-collapsible="icon">
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton isActive>
                    <Library />
                    <span>{o.sidebar.library}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <Folder />
                    <span>{o.sidebar.folders}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <Activity />
                    <span>{o.sidebar.activity}</span>
                  </SidebarMenuButton>
                  <SidebarMenuCount
                    count={3}
                    text={o.sidebar.count}
                    label={o.sidebar.countLabel}
                  />
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <Settings />
                    <span>{o.sidebar.settings}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </div>
          </div>
        </SidebarProvider>
      </Item>
    </div>
  );
}

const kindColours = [1, 2, 3, 4, 5, 6, 7, 8] as const;

/** The pieces of a record list and a record editor page, in their states. */
function RecordComponents() {
  const o = t.designSystem.overlay;
  const r = t.designSystem.pattern.record;
  const stay = (event: MouseEvent) => event.preventDefault();
  const noop = () => undefined;
  const card = (overrides: Partial<RecordCardProps>) => (
    <RecordCard
      link={<a href="#record-card" onClick={stay} />}
      name={r.tags.travel}
      count={42}
      countTitle={r.videoCount(42)}
      detail={r.kindNames.genre}
      secondary={r.synonyms.travel}
      kind={3}
      selection={{
        selected: false,
        selectionMode: false,
        label: r.select(r.tags.travel),
        onSelectedChange: noop,
      }}
      {...overrides}
    />
  );
  return (
    <>
      <Item name="RecordCard" wide>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <State label={o.states.normal}>{card({})}</State>
          <State label={o.states.hover}>
            <Demo state="hover">{card({})}</Demo>
          </State>
          <State label={o.recordCard.tentative}>
            {card({
              name: r.tags.kyoto,
              nameMarks: <TentativeMark />,
              detail: r.tentativeDetail(r.kindNames.place),
              secondary: r.synonyms.kyoto,
              kind: 1,
              tentative: true,
            })}
          </State>
          <State label={o.recordCard.plain}>
            {card({
              name: r.tags.night,
              count: 7,
              detail: r.noKind,
              secondary: [],
              kind: undefined,
            })}
          </State>
          <State label={o.states.selected}>
            {card({
              selection: {
                selected: true,
                selectionMode: true,
                label: r.select(r.tags.travel),
                onSelectedChange: noop,
              },
            })}
          </State>
          <State label={o.recordCard.longName}>
            {card({ name: o.recordCard.longNameValue, count: 1284, kind: 6 })}
          </State>
        </div>
      </Item>

      <Item name="KindMark">
        <div className="grid gap-3">
          <div className="flex flex-wrap gap-3">
            {kindColours.map((kind) => (
              <span key={kind} className="flex items-center gap-1.5 text-sm">
                <KindMark kind={kind} variant="dot" />
                {o.kindColour(kind)}
              </span>
            ))}
          </div>
          <div className="flex flex-wrap gap-2">
            {kindColours.map((kind) => (
              <span
                key={kind}
                className="relative overflow-hidden rounded-md border border-border bg-card py-2 ps-3 pe-2 text-xs"
              >
                <KindMark kind={kind} variant="bar" />
                {o.kindColour(kind)}
              </span>
            ))}
          </div>
        </div>
      </Item>

      <Item name="KindToggleGroup">
        <div className="grid gap-3">
          <State label={o.kindToggle.kind}>
            {/* The scroll layout scrolls inside its slot, as ListPage's filter does. */}
            <div className="w-full overflow-x-auto contain-inline-size">
              <KindToggleGroup
                label={r.filter}
                value="1"
                onValueChange={noop}
                options={[
                  { value: "all", label: r.all, count: 6 },
                  { value: "1", label: r.kindNames.place, kind: 1, count: 2 },
                  { value: "2", label: r.kindNames.season, kind: 2, count: 1 },
                  { value: "none", label: r.noKind, count: 1 },
                ]}
              />
            </div>
          </State>
          <State label={o.kindToggle.plain}>
            <KindToggleGroup
              label={r.kind}
              layout="wrap"
              value="none"
              onValueChange={noop}
              options={[
                { value: "1", label: r.kindNames.place, kind: 1 },
                { value: "3", label: r.kindNames.genre, kind: 3 },
                { value: "none", label: r.none },
              ]}
            />
          </State>
        </div>
      </Item>

      <Item name="TallyStrip · FloatingNotice" wide>
        <div className="grid gap-4">
          <TallyStrip
            label={r.summary}
            clearLabel={r.clear}
            tallies={[
              {
                id: "tentative",
                count: 128,
                label: r.tentative,
                hint: r.tentativeHint,
                action: r.review,
                pressed: true,
                onPressedChange: noop,
              },
              {
                id: "no-kind",
                count: 37,
                label: r.noKind,
                hint: r.noKindHint,
                action: r.sort,
                pressed: false,
                onPressedChange: noop,
              },
              {
                id: "unused",
                count: 0,
                empty: true,
                label: r.unused,
                hint: r.unusedHint,
                action: r.cleanUp,
                pressed: false,
                onPressedChange: noop,
              },
            ]}
          />
          <TallyStrip
            label={r.summary}
            clearLabel={r.clear}
            tallies={[
              {
                id: "tentative",
                label: r.tentative,
                pressed: false,
                onPressedChange: noop,
              },
              { id: "no-kind", label: r.noKind, pressed: false, onPressedChange: noop },
              { id: "unused", label: r.unused, pressed: false, onPressedChange: noop },
            ]}
          />
          <div className="flex justify-center">
            <FloatingNotice message={r.changed} actionLabel={r.refresh} onAction={noop} />
          </div>
        </div>
      </Item>
    </>
  );
}

function VvComponents() {
  const o = t.designSystem.overlay;
  const noop = () => Promise.resolve();
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Item name="FilterDropdownButton" wide>
        <div className="flex flex-wrap items-center gap-2">
          <FilterDropdownButton
            name={o.filter.status}
            value={o.filter.all}
            active={false}
          >
            <DropdownMenuRadioGroup value="all">
              <DropdownMenuRadioItem value="all">{o.filter.all}</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="running">
                {o.filter.running}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="attention">
                {o.filter.attention}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="done">{o.filter.done}</DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </FilterDropdownButton>
          <FilterDropdownButton name={o.filter.status} value={o.filter.attention} active>
            <DropdownMenuRadioGroup value="attention">
              <DropdownMenuRadioItem value="all">{o.filter.all}</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="attention">
                {o.filter.attention}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </FilterDropdownButton>
          <FilterDropdownButton name={o.filter.period} value={o.filter.today} active>
            <DropdownMenuRadioGroup value="today">
              <DropdownMenuRadioItem value="any">
                {o.filter.anyTime}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="today">
                {o.filter.today}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </FilterDropdownButton>
        </div>
      </Item>
      <Item name="VideoThumbnail" wide>
        <div className="grid gap-4 sm:grid-cols-3">
          <State label={o.states.normal}>
            <VideoThumbnail className="w-card-0 max-w-full">
              <div className="size-full bg-linear-to-br from-primary-soft to-secondary" />
              <VideoThumbnailDuration>
                <Globe />
                {o.thumbnail.duration}
              </VideoThumbnailDuration>
              <VideoThumbnailProgress value={35} aria-label={o.thumbnail.watched} />
              <VideoThumbnailMark corner="top-end">
                <FavoriteToggle
                  favorite
                  label={o.favoriteOn}
                  onToggle={noop}
                  variant="card"
                />
              </VideoThumbnailMark>
            </VideoThumbnail>
          </State>
          <State label={o.states.selected}>
            <VideoThumbnail selected className="w-card-0 max-w-full">
              <div className="size-full bg-linear-to-br from-secondary to-muted" />
              <VideoThumbnailDuration>
                <RemoteMark label={o.thumbnail.remote} />
                {o.thumbnail.duration}
              </VideoThumbnailDuration>
            </VideoThumbnail>
          </State>
          <State label={o.states.disabled}>
            <VideoThumbnail className="w-card-0 max-w-full">
              <div className="size-full bg-muted" />
              <VideoThumbnailNotice>
                <AlertTriangle />
                {t.video.unplayable.failed}
              </VideoThumbnailNotice>
            </VideoThumbnail>
          </State>
        </div>
      </Item>

      <Item name="FavoriteToggle">
        <div className="flex flex-wrap items-end gap-4">
          <State label={o.states.hover}>
            <Demo state="hover">
              <div className="group">
                <FavoriteToggle
                  favorite={false}
                  label={o.favoriteOff}
                  onToggle={noop}
                  variant="row"
                />
              </div>
            </Demo>
          </State>
          <State label={o.states.selected}>
            <FavoriteToggle favorite label={o.favoriteOn} onToggle={noop} variant="row" />
          </State>
          <State label={o.states.selected}>
            <FavoriteToggle
              favorite
              label={o.favoriteOn}
              onToggle={noop}
              variant="page"
            />
          </State>
        </div>
      </Item>

      <Item name="RateRing">
        <div className="flex flex-wrap items-end gap-4">
          <State label={o.rateRing.primary}>
            <RateRing value={96} tone="primary" />
          </State>
          <State label={o.rateRing.muted}>
            <RateRing value={74} tone="muted" />
          </State>
          <State label={o.rateRing.full}>
            <RateRing value={100} />
          </State>
        </div>
      </Item>

      <RecordComponents />

      <Item name="TentativeMark · BrandHomeLink">
        <div className="flex flex-wrap items-center gap-4">
          <Badge variant="secondary">
            {o.tentativeTag}
            <TentativeMark />
          </Badge>
          <span className="flex items-center gap-1 text-sm">
            {o.tentativeTag}
            <TentativeMark size="row" />
          </span>
          <div className="rounded-md bg-navbar p-2">
            <BrandHomeLink />
          </div>
        </div>
      </Item>
    </div>
  );
}

export default function OverlayComponents() {
  const o = t.designSystem.overlay;
  return (
    <div className="grid gap-8">
      <div className="grid gap-3">
        <h3 className="text-sm font-semibold text-muted-foreground">{o.title}</h3>
        <Overlays />
        <Feedback />
      </div>
      <div className="grid gap-3">
        <h3 className="text-sm font-semibold text-muted-foreground">{o.vvTitle}</h3>
        <VvComponents />
      </div>
    </div>
  );
}
