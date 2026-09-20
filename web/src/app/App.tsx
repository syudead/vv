import { BrowserRouter, Route, Routes } from "react-router";

import LibraryPage from "../library/LibraryPage";
import VideoPage from "../player/VideoPage";
import AppShell from "../shell/AppShell";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";

/**
 * App は画面の割り当てである。
 *
 * 一覧だけをシェル（トップバー + サイドバー）で包み、再生画面はシアターモード
 * として包まない。この分岐はここ 1 か所に閉じる。
 */
export default function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <ToastProvider>
          <ScanProvider>
            <Routes>
              <Route
                path="/"
                element={
                  <AppShell>
                    <LibraryPage />
                  </AppShell>
                }
              />
              <Route path="/videos/:id" element={<VideoPage />} />
            </Routes>
          </ScanProvider>
        </ToastProvider>
      </TooltipProvider>
    </BrowserRouter>
  );
}
