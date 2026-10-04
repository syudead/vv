import {
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
import type { ReactNode } from "react";
import { toast } from "sonner";

import { t } from "../i18n";
import { Alert, AlertDescription, AlertTitle } from "../ui/alert";
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
} from "../ui/alert-dialog";
import { Badge } from "../ui/badge";
import BrandHomeLink from "../ui/BrandHomeLink";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "../ui/breadcrumb";
import Button from "../ui/Button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "../ui/dialog";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/dropdown-menu";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "../ui/empty";
import FavoriteToggle from "../ui/FavoriteToggle";
import { Kbd, KbdGroup } from "../ui/kbd";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "../ui/popover";
import { Progress } from "../ui/progress";
import { Separator } from "../ui/separator";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "../ui/sidebar";
import { Skeleton } from "../ui/skeleton";
import { Toaster } from "../ui/sonner";
import { Spinner } from "../ui/spinner";
import { Tabs, TabsList, TabsTrigger } from "../ui/tabs";
import TentativeMark from "../ui/TentativeMark";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailMark,
  VideoThumbnailNotice,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";

// 見本の「Overlays and feedback」と「vv components」の節（specs/038-design-system/ui-design.md
// 「Components」「Review criteria」4）。部品ごとに、通常・hover・キーボードのフォーカス・押下・
// 選択・無効のうち持つ状態を並べる。hover などは index.css の data-state-preview で固定する。

type Preview = "hover" | "focus" | "active";

/** 検索の書き方の記号（videoList/SearchSyntaxHelp と同じ。訳さない）。 */
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

/** State は 1 つの状態の見本に、その状態の名前を添える。 */
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
            <Button variant="secondary" size="sm">
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
              <Button variant="secondary" size="sm">
                {t.common.cancel}
              </Button>
              <Button variant="primary" size="sm">
                {o.merge}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </Item>

      <Item name="AlertDialog">
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="secondary" size="sm">
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
              <DropdownMenuItem data-state-preview="focus">
                <Lock />
                {o.makePrivate}
                <span className="ml-auto text-xs text-muted-foreground">
                  {o.states.hover}
                </span>
              </DropdownMenuItem>
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
              <TabsTrigger value="tentative" data-state-preview="hover">
                {o.tabs.tentative}
              </TabsTrigger>
              <TabsTrigger value="rejected" disabled>
                {o.tabs.rejected}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <Tabs defaultValue="all">
            <TabsList variant="line">
              <TabsTrigger value="all">{o.tabs.all}</TabsTrigger>
              <TabsTrigger value="tentative" data-state-preview="focus">
                {o.tabs.tentative}
              </TabsTrigger>
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
              <Badge asChild variant="secondary">
                <a href="#components" data-state-preview={state.preview}>
                  {o.badges.tag}
                  <TentativeMark />
                </a>
              </Badge>
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
              <BreadcrumbLink href="#components" data-state-preview="hover">
                {o.breadcrumb.parent}
              </BreadcrumbLink>
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
                      <SidebarMenuButton data-state-preview={state.preview}>
                        <Folder />
                        <span>{`${o.sidebar.folders} · ${state.label}`}</span>
                      </SidebarMenuButton>
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

function VvComponents() {
  const o = t.designSystem.overlay;
  const noop = () => Promise.resolve();
  return (
    <div className="grid gap-4 lg:grid-cols-2">
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
              <VideoThumbnailDuration>{o.thumbnail.duration}</VideoThumbnailDuration>
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
            <div className="group" data-state-preview="hover">
              <FavoriteToggle
                favorite={false}
                label={o.favoriteOff}
                onToggle={noop}
                variant="row"
              />
            </div>
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

/**
 * OverlayComponents は見本の部品の節のうち、重ね表示と通知、vv 固有部品を並べる
 * （Issue 771）。操作と入力の部品は別の単位が同じ節に並べる。
 */
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
