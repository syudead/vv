package main

import (
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/eventbus"
)

// waker は1つの段階のワーカーを起こす。internal/jobs の *Worker がこれを満たす。
type waker interface {
	Wake()
}

// eventSubscribers は状態の変化を受け取る側である。発行する側（保存層・
// 取り込み・走査）はこれらを知らない。
type eventSubscribers struct {
	// Screen は画面へ送る知らせ（/api/events）へ変化を渡す。動画の上書きの変化
	// （domain.VideoOverrideChanged）もここで video の知らせになり、ワーカーの起床には
	// 結ばない（specs/029-video-overrides/research.md R-7）。束ねの変化
	// （domain.VideoBundleChanged）は、影響した各動画の video の知らせになる
	// （specs/030-video-versions/research.md R-9）。
	Screen func(domain.Event)
	// Workers は段階ごとのワーカーである。
	Workers map[domain.JobKind]waker
	// ReleaseArtifacts は参照の無くなった内容の生成物を消す。
	ReleaseArtifacts func(domain.ContentUnreferenced)
}

// eventSubscriptions は、停止の順番に合わせて購読をやめる関数である。
type eventSubscriptions struct {
	// StopScreen は画面への知らせをやめる。閉じた接続へは書かない。
	StopScreen func()
	// StopWorkers はワーカーを起こすのをやめる。止めたワーカーは起こさない。
	StopWorkers func()
}

// subscribeEvents は、どの変化で何が起こるかを1か所で登録する。購読者を
// 増やすときはここに加え、発行する側は変えない。
//
// 生成物の削除は購読をやめない。bus.Close が、積んである分を渡し終えるまで
// 待つ。途中でやめると、消すはずの生成物が残り続ける。
func subscribeEvents(bus *eventbus.Bus, s eventSubscribers) eventSubscriptions {
	var stops eventSubscriptions
	stops.StopScreen = func() {}
	if s.Screen != nil {
		stops.StopScreen = bus.Subscribe("screen notifications", s.Screen)
	}

	var stopWorkers []func()
	for kind, worker := range s.Workers {
		// 仕事が積まれた段階のワーカーを起こす。ワーカーは待ち行列を一定間隔で
		// 問い合わせない。
		stopWorkers = append(stopWorkers, eventbus.On(bus, "worker wake-up ("+string(kind)+")",
			func(event domain.JobsQueued) {
				if slices.Contains(event.Kinds, kind) {
					worker.Wake()
				}
			}))
	}
	// サムネイルは解析が終わるまで取り出されない（抽出位置が動画の長さで
	// 決まる）。解析の成否が決まったら、待っていたサムネイルのワーカーを起こす。
	if thumbnail, ok := s.Workers[domain.JobThumbnail]; ok {
		stopWorkers = append(stopWorkers, eventbus.On(bus, "thumbnail after probe",
			func(event domain.VideoIngestChanged) {
				if event.Stage == domain.JobProbe {
					thumbnail.Wake()
				}
			}))
	}
	// シーク用サムネイルは、解析が終わり、取り出せる代表サムネイルの仕事が無い
	// ときだけ取り出される（domain.ClaimConditionFor）。解析か代表サムネイルの
	// 1件の成否が決まったとき、または動画の行が消えたとき（Stage が空。残りの
	// 代表サムネイルの仕事が減る）に、待っていたシーク用のワーカーを起こす。
	// 条件は取り出しの時点だけで効き、走っているシーク用の生成は止めない。
	if seek, ok := s.Workers[domain.JobSeekThumbnail]; ok {
		stopWorkers = append(stopWorkers, eventbus.On(bus, "seek thumbnail after thumbnail",
			func(event domain.VideoIngestChanged) {
				switch event.Stage {
				case domain.JobProbe, domain.JobThumbnail, "":
					seek.Wake()
				}
			}))
	}
	// 指紋は完成したシーク用スプライトを待つ（domain.ClaimConditionFor）が、その仕事は
	// シーク用サムネイルの完了を書く取引で積まれる（JobsQueued）ので、ほかの段階の成否で
	// 起こし直す必要は無い。
	stops.StopWorkers = func() {
		for _, stop := range stopWorkers {
			stop()
		}
	}

	if s.ReleaseArtifacts != nil {
		eventbus.On(bus, "artifact release", s.ReleaseArtifacts)
	}
	return stops
}
