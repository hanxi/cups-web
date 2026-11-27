package main

import (
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// passkey 的 begin → finish 之间需要保留 WebAuthn SessionData（含 challenge）。
// 仅存进程内存：一次性消费、5 分钟过期、总量设上限，防止匿名请求刷爆内存。
const (
	passkeyChallengeTTL = 5 * time.Minute
	passkeyChallengeMax = 2000
)

type passkeyChallenge struct {
	kind    string // "register" | "login"
	userID  int64  // register 时为发起用户；login 为 0
	session webauthn.SessionData
	expires time.Time
}

type passkeyChallengeStore struct {
	mu    sync.Mutex
	items map[string]passkeyChallenge
	now   func() time.Time
}

func newPasskeyChallengeStore() *passkeyChallengeStore {
	return &passkeyChallengeStore{items: make(map[string]passkeyChallenge), now: time.Now}
}

// put 保存挑战并返回不透明 key；容量已满（清理过期项后仍满）时返回 false。
func (s *passkeyChallengeStore) put(c passkeyChallenge) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, v := range s.items {
		if now.After(v.expires) {
			delete(s.items, k)
		}
	}
	if len(s.items) >= passkeyChallengeMax {
		return "", false
	}
	c.expires = now.Add(passkeyChallengeTTL)
	key := randomToken()
	s.items[key] = c
	return key, true
}

// take 取出并删除（一次性）；过期、不存在或 kind 不符都返回 false。
func (s *passkeyChallengeStore) take(key, kind string) (passkeyChallenge, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.items[key]
	if !ok {
		return passkeyChallenge{}, false
	}
	delete(s.items, key)
	if s.now().After(c.expires) || c.kind != kind {
		return passkeyChallenge{}, false
	}
	return c, true
}
