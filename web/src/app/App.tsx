import { BrowserRouter, Route, Routes, useLocation } from "react-router";

import LibraryPage from "../library/LibraryPage";
import VideoPage from "../player/VideoPage";
import AppShell from "../shell/AppShell";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import ScanProgressIndicator from "../shell/ScanProgressIndicator";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import { deferredRoute } from "./deferredRoute";

// 一覧と再生画面のほかは使うときだけ読み込む（deferredRoute）。再生画面から
// 一覧へ戻るたびの読み込みに、ほかの画面の部品を加えない。
const FolderPage = await deferredRoute(() => import("../folders/FolderPage"), "/folders");
const SettingsPage = await deferredRoute(
  () => import("../settings/SettingsPage"),
  "/settings",
);
const TagsPage = await deferredRoute(() => import("../tags/TagsPage"), "/tags");

function AppRoutes() {
  const location = useLocation();
  const scanPlacement = location.pathname.startsWith("/videos/") ? "playback" : "default";

  return (
    <ToastProvider placement={scanPlacement}>
      <ScanProgressIndicator />
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
        <Route
          path="/settings"
          element={
            <AppShell>
              <SettingsPage />
            </AppShell>
          }
        />
        <Route
          path="/tags"
          element={
            <AppShell>
              <TagsPage />
            </AppShell>
          }
        />
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
    </ToastProvider>
  );
}

/**
 * App は画面の割り当てである。
 *
 * 一覧・フォルダ・設定をシェル（トップバー + サイドバー）で包み、再生画面は
 * シアターモードとして包まない。この分岐はここ 1 か所に閉じる。
 */
export default function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <ScanProvider>
          <ScanNoticeProvider>
            <AppRoutes />
          </ScanNoticeProvider>
        </ScanProvider>
      </TooltipProvider>
    </BrowserRouter>
  );
}
