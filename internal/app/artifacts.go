package app

import (
	"context"
	"log/slog"
	"sync"
)

// artifactKind は生成物の種類である。種類ごとに生成を直列にする。
type artifactKind int

const (
	artifactThumbnail artifactKind = iota
	artifactSeekThumbnails
	artifactPreview
)

// artifacts は内容ごとの生成物（代表サムネイル・シーク用プレビュー・一覧用
// プレビュー）の削除と、作成との直列化を受け持つ。
//
// 作成と削除は内容の識別子ごとの読み書き錠で直列にする。錠が無いと、削除の側が
// 「参照が無い」と確かめてからファイルを消すまでの間に同じ内容の動画が取り込まれ、
// その動画のジョブが既存のファイルを採用して完了にした直後に、ファイルだけが
// 消えることがある。錠の中で「参照を確かめて消す」と「生成して完了を記録する」を
// 行えば、どちらが先でも状態とファイルが食い違わない。
//
// 生成は内容の錠を共有で、削除は専有で持つ。種類の違う生成は互いを待たず、
// 同じ種類の生成（重複ファイルの別の動画など）は内容と種類の組ごとの錠で直列にする。
// 錠は必ず種類、内容の順に取る。
type artifacts struct {
	index  ContentIndex
	files  ArtifactRemover
	logger *slog.Logger

	contents keyedLocks[string]
	kinds    keyedLocks[artifactKey]

	// releasing は背後で動いている削除である。停止時に待つ。
	releasing sync.WaitGroup
}

type artifactKey struct {
	contentKey string
	kind       artifactKind
}

func newArtifacts(index ContentIndex, files ArtifactRemover, logger *slog.Logger) *artifacts {
	return &artifacts{index: index, files: files, logger: logger}
}

// lockGeneration は内容と種類の組の錠を取り、内容の錠を共有で取って、外す関数を
// 返す。種類の違う生成とは並んで進み、削除とは重ならない。
func (a *artifacts) lockGeneration(contentKey string, kind artifactKind) func() {
	unlockKind := a.kinds.lock(artifactKey{contentKey: contentKey, kind: kind})
	unlockContent := a.contents.rlock(contentKey)
	return func() {
		unlockContent()
		unlockKind()
	}
}

// generate は、生成（既存のファイルの採用を含む）から完了の記録までの run を
// 同じ内容の削除と重ならないように行う。run が後始末を求めたら、錠を外して
// から参照の無くなった生成物を消す。後始末は削除として専有の錠で行うので、
// 同じ内容の別の種類の生成の完了の記録とは重ならない。
func (a *artifacts) generate(
	ctx context.Context, contentKey string, kind artifactKind, run func() (cleanup bool, err error),
) error {
	cleanup, err := func() (bool, error) {
		unlock := a.lockGeneration(contentKey, kind)
		defer unlock()
		return run()
	}()
	if cleanup {
		if cleanupErr := a.removeIfUnreferencedAfterGeneration(context.WithoutCancel(ctx), contentKey); cleanupErr != nil {
			return cleanupErr
		}
	}
	return err
}

// removeIfUnreferencedAfterGeneration は生成の後始末として、参照が無ければ
// 生成物を消す。まだ参照があれば錠を取らずに戻り、同じ内容の別の種類の生成を
// 待たない。その後に参照が無くなれば、動画を消した側の release が消す。
func (a *artifacts) removeIfUnreferencedAfterGeneration(ctx context.Context, contentKey string) error {
	referenced, err := a.index.ContentKeyReferenced(ctx, contentKey)
	if err != nil || referenced {
		return err
	}
	return a.removeIfUnreferenced(ctx, contentKey)
}

// removeIfUnreferencedLocked は、内容を参照する動画が無ければその生成物を消す。
// 呼び出し側がその内容の錠を専有で持っていること。
func (a *artifacts) removeIfUnreferencedLocked(ctx context.Context, contentKey string) error {
	referenced, err := a.index.ContentKeyReferenced(ctx, contentKey)
	if err != nil {
		return err
	}
	if referenced {
		return nil
	}
	return a.files.RemoveContent(contentKey)
}

// removeIfUnreferenced は内容の錠を専有で取ってから removeIfUnreferencedLocked を行う。
func (a *artifacts) removeIfUnreferenced(ctx context.Context, contentKey string) error {
	unlock := a.contents.lock(contentKey)
	defer unlock()
	return a.removeIfUnreferencedLocked(ctx, contentKey)
}

// release は、動画の行が消えたときに、参照の無くなった内容の生成物を消す。
// 消えた動画の分だけを見るので、ライブラリ全体は読まない。
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
				a.logger.Warn("could not delete artifacts of a removed video",
					slog.String("contentKey", key), slog.Any("error", err))
			}
		}
	}()
}

// wait は背後で動いている削除の終わりを待つ。
func (a *artifacts) wait() {
	a.releasing.Wait()
}

// keyedLocks は鍵ごとの読み書き錠である。使い終わった錠は捨てる。
type keyedLocks[K comparable] struct {
	mu    sync.Mutex
	locks map[K]*keyedLock
}

type keyedLock struct {
	sync.RWMutex
	users int
}

// lock は鍵の錠を専有で取り、外す関数を返す。
func (l *keyedLocks[K]) lock(key K) func() {
	entry := l.acquire(key)
	entry.Lock()
	return func() {
		entry.Unlock()
		l.release(key, entry)
	}
}

// rlock は鍵の錠を共有で取り、外す関数を返す。
func (l *keyedLocks[K]) rlock(key K) func() {
	entry := l.acquire(key)
	entry.RLock()
	return func() {
		entry.RUnlock()
		l.release(key, entry)
	}
}

func (l *keyedLocks[K]) acquire(key K) *keyedLock {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locks == nil {
		l.locks = map[K]*keyedLock{}
	}
	entry, ok := l.locks[key]
	if !ok {
		entry = &keyedLock{}
		l.locks[key] = entry
	}
	entry.users++
	return entry
}

func (l *keyedLocks[K]) release(key K, entry *keyedLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.users--
	if entry.users == 0 {
		delete(l.locks, key)
	}
}
