package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/store"
)

// artifacts は内容ごとの生成物（代表サムネイル・シーク用プレビュー・一覧用
// プレビュー）の作成と削除を受け持つ。
//
// 作成と削除は内容の識別子ごとの錠で直列にする。錠が無いと、削除の側が
// 「参照が無い」と確かめてからファイルを消すまでの間に同じ内容の動画が取り込まれ、
// その動画のジョブが既存のファイルを採用して完了にした直後に、ファイルだけが
// 消えることがある。錠の中で「参照を確かめて消す」と「生成して完了を記録する」を
// 行えば、どちらが先でも状態とファイルが食い違わない。
type artifacts struct {
	db            *store.DB
	thumbnailsDir string
	logger        *slog.Logger

	mu    sync.Mutex
	locks map[string]*artifactLock

	// releasing は背後で動いている削除である。停止時に待つ。
	releasing sync.WaitGroup
}

type artifactLock struct {
	sync.Mutex
	users int
}

func newArtifacts(db *store.DB, thumbnailsDir string, logger *slog.Logger) *artifacts {
	return &artifacts{
		db:            db,
		thumbnailsDir: thumbnailsDir,
		logger:        logger,
		locks:         map[string]*artifactLock{},
	}
}

// lock は内容の識別子の錠を取り、外す関数を返す。使い終わった錠は捨てる。
func (a *artifacts) lock(contentKey string) func() {
	a.mu.Lock()
	entry, ok := a.locks[contentKey]
	if !ok {
		entry = &artifactLock{}
		a.locks[contentKey] = entry
	}
	entry.users++
	a.mu.Unlock()

	entry.Lock()
	return func() {
		entry.Unlock()
		a.mu.Lock()
		entry.users--
		if entry.users == 0 {
			delete(a.locks, contentKey)
		}
		a.mu.Unlock()
	}
}

// removeIfUnreferencedLocked は、内容を参照する動画が無ければその生成物を消す。
// 呼び出し側がその内容の錠を持っていること。
func (a *artifacts) removeIfUnreferencedLocked(ctx context.Context, contentKey string) error {
	referenced, err := a.db.ContentKeyReferenced(ctx, contentKey)
	if err != nil {
		return err
	}
	if referenced {
		return nil
	}
	return media.RemoveContentArtifacts(a.thumbnailsDir, contentKey)
}

// removeIfUnreferenced は錠を取ってから removeIfUnreferencedLocked を行う。
func (a *artifacts) removeIfUnreferenced(ctx context.Context, contentKey string) error {
	unlock := a.lock(contentKey)
	defer unlock()
	return a.removeIfUnreferencedLocked(ctx, contentKey)
}

// release は、動画の行が消えたときに、参照の無くなった内容の生成物を消す。
// 保存層の OnContentReleased に渡す。消えた動画の分だけを見るので、
// ライブラリ全体は読まない。
//
// ファイルの削除は呼び出し元（走査やフォルダ設定の要求）を待たせないよう背後で
// 行い、停止時は wait で終わりを待つ。
func (a *artifacts) release(contentKeys []string) {
	a.releasing.Add(1)
	go func() {
		defer a.releasing.Done()
		ctx := context.Background()
		for _, key := range contentKeys {
			if err := a.removeIfUnreferenced(ctx, key); err != nil {
				a.logger.Warn("消えた動画の生成物を削除できませんでした",
					slog.String("contentKey", key), slog.Any("error", err))
			}
		}
	}()
}

// wait は背後で動いている削除の終わりを待つ。データベースを閉じる前に呼ぶ。
func (a *artifacts) wait() {
	a.releasing.Wait()
}
