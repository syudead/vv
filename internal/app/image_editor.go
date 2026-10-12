package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// ImageEditor は画像編集の提供元を動かすポートである。入力を上げて実行を始め、状態を読み、
// 中止し、出力を取る。internal/comfyui の *Client が自前の ComfyUI と Comfy Cloud の両方で
// 満たす（specs/045-comfyui-thumbnails/research.md R-1・R-5）。提供元の失敗は
// *domain.ImageEditError で返す。
type ImageEditor interface {
	// Start は接続先 conn に入力を上げ、vv のワークフローを送って実行を始める。
	Start(ctx context.Context, conn domain.ComfyUISettings, input domain.ImageEditInput) (domain.ImageEditJob, error)
	// State は実行の状態を返す。提供元の状態が既知のどれにも当たらなければ previous を返す。
	// failed のときは理由を *domain.ImageEditError で添える。
	State(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error)
	// Cancel は実行を中止する。state はその実行の今の状態で、running なら実行を止め、
	// それより前なら待ち行列から消す。
	Cancel(ctx context.Context, job domain.ImageEditJob, state domain.ThumbnailEditState) error
	// Output は成功した実行の出力の画像を返す。
	Output(ctx context.Context, job domain.ImageEditJob) ([]byte, error)
}
