import { BrowserRouter, Route, Routes } from "react-router";

import AppShell from "./layout/AppShell";
import LibraryPage from "./pages/LibraryPage";
import VideoPage from "./pages/VideoPage";

/**
 * App は画面の割り当てである（R-113）。
 *
 * 画面が2つになり、再生中の動画を URL で開ける（共有・再読み込みできる）
 * 必要が出たので、ここで最小のルータを入れている。宣言的モードだけを使い、
 * データ取得はルータに寄せない。
 *
 * **骨格を着せるかどうかの分岐はこの 1 か所に閉じる**（FR-015 / R-505）。
 * 一覧だけを AppShell で包み、再生画面にはサイドバーもヘッダーも被せない。
 * ここで分けておけば、骨格の側が「いまどの画面か」を知らずに済む。
 */
export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/"
          element={
            <AppShell>
              <LibraryPage />
            </AppShell>
          }
        />
        {/* 再生中の動画を URL で開ける（共有・再読み込みできる）。
            骨格で包まない — 再生画面は映像と情報パネルの 2 分割である。 */}
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
    </BrowserRouter>
  );
}
