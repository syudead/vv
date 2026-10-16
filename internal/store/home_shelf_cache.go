package store

// ホームの棚の項目の保持（specs/057-home/research.md R-12）。棚の項目はライブラリの項目の問い合わせで
// 読むので、2 万本では画面に入った棚を読むのに 1 秒を超える。読んだ棚を、棚の鍵と見る人ごとに
// homeShelfCacheTTL のあいだ保持し、ホームの画面を読み直させる知らせ（動画・グループ・スマート
// フォルダ・ホームの変化）を発行するたびに捨てる。知らせの無い変化（再生の記録など）は、おすすめと
// 同じく保持のあいだ前の値のままである。

import (
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// homeShelfCacheTTL は棚の項目を保持する長さである。
const homeShelfCacheTTL = 10 * time.Minute

// homeShelfCacheNow は保持の時刻の元である。試験が時計を進めるために差し替える。
var homeShelfCacheNow = time.Now

// homeShelfCacheKey は保持する棚 1 つの鍵である。
type homeShelfCacheKey struct {
	shelf    domain.HomeShelfKey
	audience domain.Audience
}

// homeShelfCacheEntry は保持した棚 1 つの読み出しの結果である。shown が偽なら、その鍵は応答から
// 落とす（もう無い棚・ゲストに見せない棚）。isShelf は鍵が今の棚を名指すかで、偽の結果は保持しない。
// 棚でない鍵（無いフォルダの id など）は見る人がいくらでも作れ、保持すると保持が増え続けるからである。
// 棚の鍵は棚の数（domain.MaxHomeShelves）と決まった棚に限られる。
type homeShelfCacheEntry struct {
	shelf   domain.HomeShelfItems
	shown   bool
	isShelf bool
	expiry  time.Time
}

// homeShelfCache は棚の項目の保持である。generation は捨てるたびに進み、捨てる前に読み始めた結果を
// 捨てた後にしまわない。
type homeShelfCache struct {
	mu         sync.Mutex
	entries    map[homeShelfCacheKey]homeShelfCacheEntry
	generation uint64
}

// get は鍵 key の保持した結果を返す。無いか期限が過ぎていれば ok は偽である。
func (c *homeShelfCache) get(key homeShelfCacheKey) (homeShelfCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !homeShelfCacheNow().Before(entry.expiry) {
		return homeShelfCacheEntry{}, false
	}
	return entry, true
}

// start は読み始める時点の generation を返す。
func (c *homeShelfCache) start() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

// put は generation の時点から読んだ結果 entry をしまう。その後に捨てていたか、entry が棚の結果で
// なければ何もしない。
func (c *homeShelfCache) put(generation uint64, key homeShelfCacheKey, entry homeShelfCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation || !entry.isShelf {
		return
	}
	if c.entries == nil {
		c.entries = map[homeShelfCacheKey]homeShelfCacheEntry{}
	}
	entry.expiry = homeShelfCacheNow().Add(homeShelfCacheTTL)
	c.entries[key] = entry
}

// dropFor は events のどれかがホームの画面を読み直させる知らせなら、保持したすべてを捨てる。
func (c *homeShelfCache) dropFor(events []domain.Event) {
	for _, event := range events {
		if reloadsHome(event) {
			c.mu.Lock()
			c.generation++
			clear(c.entries)
			c.mu.Unlock()
			return
		}
	}
}

// reloadsHome は event が画面へ video・group・smartFolder・home の知らせとして届くか
// （internal/httpapi の Events.Handle）を返す。
func reloadsHome(event domain.Event) bool {
	switch event.(type) {
	case domain.VideoIngestChanged, domain.VideoOverrideChanged, domain.VideoBundleChanged, domain.VideoWorkInfoChanged,
		domain.AutoTagApplied, domain.GroupsChanged, domain.SmartFoldersChanged, domain.HomeChanged:
		return true
	}
	return false
}
