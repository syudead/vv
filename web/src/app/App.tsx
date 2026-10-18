import { lazy, Suspense } from "react";
import { BrowserRouter, Route, Routes, useLocation } from "react-router";

import { useAudience } from "../auth/audience";
import AuthGate from "../auth/AuthGate";
import LibraryPage from "../library/LibraryPage";
import VideoPage from "../player/VideoPage";
import AppShell from "../shell/AppShell";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import ScanProgressIndicator from "../shell/ScanProgressIndicator";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { isVideoEditPath } from "../lib/videoEditPath";
import { deferredRoute, matchesBelowPath, matchesExactPath } from "./deferredRoute";

// Load secondary routes on demand so returning to the list does not load unrelated screens.
const FolderPage = await deferredRoute(() => import("../folders/FolderPage"), "/folders");
const SettingsPage = await deferredRoute(
  () => import("../settings/SettingsPage"),
  "/settings",
);
const TagsPage = await deferredRoute(() => import("../tags/TagsPage"), "/tags");
const PeoplePage = await deferredRoute(() => import("../credits/PeoplePage"), "/people");
const OrganizationsPage = await deferredRoute(
  () => import("../credits/OrganizationsPage"),
  "/organizations",
);
const HistoryPage = await deferredRoute(
  () => import("../history/HistoryPage"),
  "/history",
);
// These routes share a prefix; await only the directly opened page.
const PlaylistsPage = await deferredRoute(
  () => import("../playlists/PlaylistsPage"),
  (pathname) => matchesExactPath(pathname, "/playlists"),
);
const PlaylistPage = await deferredRoute(
  () => import("../playlists/PlaylistPage"),
  (pathname) => matchesBelowPath(pathname, "/playlists"),
);
const GroupPage = await deferredRoute(() => import("../groups/GroupPage"), "/groups");
const SmartFolderPage = await deferredRoute(
  () => import("../smartFolders/SmartFolderPage"),
  "/smart-folders",
);
const DuplicatesPage = await deferredRoute(
  () => import("../versions/DuplicatesPage"),
  "/duplicates",
);
const ActivityPage = await deferredRoute(
  () => import("../activity/ActivityPage"),
  "/activity",
);
// Keep editor dependencies out of playback loading.
const VideoDetailsEditor = await deferredRoute(
  () => import("../player/detailsEditor/VideoDetailsEditor"),
  isVideoEditPath,
);
const SetupPage = await deferredRoute(() => import("../auth/SetupPage"), "/setup");
const LoginPage = await deferredRoute(() => import("../auth/LoginPage"), "/login");
// The production build replaces import.meta.env.DEV with false and removes this import.
const DesignSystemPage = import.meta.env.DEV
  ? lazy(() => import("../designSystem/DesignSystemPage"))
  : null;

function AppRoutes() {
  const location = useLocation();
  const owner = useAudience() === "owner";
  const scanPlacement = location.pathname.startsWith("/videos/") ? "playback" : "default";

  return (
    <ToastProvider placement={scanPlacement}>
      {/* Only owners receive import progress and notifications. */}
      {owner && <ScanProgressIndicator />}
      <Routes>
        <Route
          path="/"
          element={
            <AppShell>
              <LibraryPage />
            </AppShell>
          }
        />
        {["/folders", "/folders/*"].map((path) => (
          <Route
            key={path}
            path={path}
            element={
              <AppShell>
                <FolderPage />
              </AppShell>
            }
          />
        ))}
        {["/settings", "/settings/*"].map((path) => (
          <Route
            key={path}
            path={path}
            element={
              <AppShell>
                <SettingsPage />
              </AppShell>
            }
          />
        ))}
        <Route
          path="/tags"
          element={
            <AppShell>
              <TagsPage />
            </AppShell>
          }
        />
        <Route
          path="/people"
          element={
            <AppShell>
              <PeoplePage />
            </AppShell>
          }
        />
        <Route
          path="/organizations"
          element={
            <AppShell>
              <OrganizationsPage />
            </AppShell>
          }
        />
        <Route
          path="/history"
          element={
            <AppShell>
              <HistoryPage />
            </AppShell>
          }
        />
        <Route
          path="/playlists"
          element={
            <AppShell>
              <PlaylistsPage />
            </AppShell>
          }
        />
        <Route
          path="/playlists/:id"
          element={
            <AppShell>
              <PlaylistPage />
            </AppShell>
          }
        />
        <Route
          path="/groups/:id"
          element={
            <AppShell>
              <GroupPage />
            </AppShell>
          }
        />
        <Route
          path="/smart-folders/:id"
          element={
            <AppShell>
              <SmartFolderPage />
            </AppShell>
          }
        />
        <Route
          path="/duplicates"
          element={
            <AppShell>
              <DuplicatesPage />
            </AppShell>
          }
        />
        <Route
          path="/activity"
          element={
            <AppShell>
              <ActivityPage />
            </AppShell>
          }
        />
        <Route path="/videos/:id" element={<VideoPage />} />
        {/* Like playback, the editor sits outside the shell. */}
        <Route path="/videos/:id/edit" element={<VideoDetailsEditor />} />
        {DesignSystemPage && (
          <Route
            path="/design-system"
            element={
              <Suspense fallback={null}>
                <DesignSystemPage />
              </Suspense>
            }
          />
        )}
      </Routes>
    </ToastProvider>
  );
}

function LibraryApp() {
  return (
    <TooltipProvider>
      <ScanProvider>
        <ScanNoticeProvider>
          <AppRoutes />
        </ScanNoticeProvider>
      </ScanProvider>
    </TooltipProvider>
  );
}

/**
 * Wait for AuthGate before rendering routes. Setup and login omit providers and the shell;
 * playback omits the shell.
 */
export default function App() {
  return (
    <BrowserRouter>
      <AuthGate>
        <Routes>
          <Route path="/setup" element={<SetupPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="*" element={<LibraryApp />} />
        </Routes>
      </AuthGate>
    </BrowserRouter>
  );
}
