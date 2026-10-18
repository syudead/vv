import { lazy, Suspense } from "react";
import { BrowserRouter, Route, Routes, useLocation } from "react-router";

import { useAudience } from "../auth/audience";
import AuthGate from "../auth/AuthGate";
import { LIBRARY_PATH } from "../library/libraryPath";
import LibraryPage from "../library/LibraryPage";
import VideoPage from "../player/VideoPage";
import AppShell from "../shell/AppShell";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import ScanProgressIndicator from "../shell/ScanProgressIndicator";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { isPersonEditPath } from "../lib/personEditPath";
import { isVideoEditPath } from "../lib/videoEditPath";
import { OnboardingGate } from "../onboarding/OnboardingGate";
import { ONBOARDING_PATH } from "../onboarding/onboardingRoute";
import { deferredRoute, matchesBelowPath, matchesExactPath } from "./deferredRoute";
import LibraryRedirect from "./LibraryRedirect";
const FolderPage = await deferredRoute(() => import("../folders/FolderPage"), "/folders");
const SettingsPage = await deferredRoute(
  () => import("../settings/SettingsPage"),
  "/settings",
);
const TagsPage = await deferredRoute(() => import("../tags/TagsPage"), "/tags");
const PeoplePage = await deferredRoute(
  () => import("../credits/PeoplePage"),
  (pathname) => matchesExactPath(pathname, "/people"),
);
const PersonPage = await deferredRoute(
  () => import("../credits/person/PersonPage"),
  (pathname) => matchesBelowPath(pathname, "/people") && !isPersonEditPath(pathname),
);
const PersonEditPage = await deferredRoute(
  () => import("../credits/person/edit/PersonEditPage"),
  isPersonEditPath,
);
const OrganizationsPage = await deferredRoute(
  () => import("../credits/OrganizationsPage"),
  "/organizations",
);
const HistoryPage = await deferredRoute(
  () => import("../history/HistoryPage"),
  "/history",
);
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
const VideoDetailsEditor = await deferredRoute(
  () => import("../player/detailsEditor/VideoDetailsEditor"),
  isVideoEditPath,
);
const OnboardingPage = await deferredRoute(
  () => import("../onboarding/OnboardingPage"),
  ONBOARDING_PATH,
);
const SetupPage = await deferredRoute(() => import("../auth/SetupPage"), "/setup");
const LoginPage = await deferredRoute(() => import("../auth/LoginPage"), "/login");
const DesignSystemPage = import.meta.env.DEV
  ? lazy(() => import("../designSystem/DesignSystemPage"))
  : null;

function AppRoutes() {
  const location = useLocation();
  const owner = useAudience() === "owner";
  const scanPlacement = location.pathname.startsWith("/videos/") ? "playback" : "default";

  return (
    <ToastProvider placement={scanPlacement}>
      {owner && <ScanProgressIndicator />}
      {/* 所有者の段階が done でない間は、どの経路も /welcome へ置き換える
          （docs/design-docs/home.md）。 */}
      <OnboardingGate>
        <Routes>
          <Route path="/library" element={<LibraryRedirect />} />
          <Route
            path={LIBRARY_PATH}
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
            path="/people/:id"
            element={
              <AppShell>
                <PersonPage />
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
          <Route path={ONBOARDING_PATH} element={<OnboardingPage />} />
          <Route path="/videos/:id" element={<VideoPage />} />
          <Route path="/videos/:id/edit" element={<VideoDetailsEditor />} />
          <Route path="/people/:id/edit" element={<PersonEditPage />} />
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
      </OnboardingGate>
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
