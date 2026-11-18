package server

import (
	"sync"
	"time"
)

// 登录失败限流器：IP + 账号双维度滑动窗口。进程内存态，单实例部署足够。
// 双维度的意义：只按账号锁会被用来 DoS 合法用户；只按 IP 锁挡不住分布式低速。
type loginBucket struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{buckets: map[string]*loginBucket{}}
}

type loginPolicy struct {
	enabled       bool
	userThreshold int
	ipThreshold   int
	window        time.Duration
	cooldown      time.Duration
}

func (s *Server) loginPolicy() loginPolicy {
	pol := loginPolicy{
		enabled:       true,
		userThreshold: 10,
		ipThreshold:   30,
		window:        15 * time.Minute,
		cooldown:      15 * time.Minute,
	}
	if s.m == nil || s.m.pg == nil {
		return pol
	}
	pg := s.m.pg
	pol.enabled = pg.GetBool("auth.lockout_enabled", true)
	if v := pg.GetInt("auth.lockout_user_threshold", 0); v > 0 {
		pol.userThreshold = v
	}
	if v := pg.GetInt("auth.lockout_ip_threshold", 0); v > 0 {
		pol.ipThreshold = v
	}
	if v := pg.GetInt("auth.lockout_window_minutes", 0); v > 0 {
		pol.window = time.Duration(v) * time.Minute
	}
	if v := pg.GetInt("auth.lockout_cooldown_minutes", 0); v > 0 {
		pol.cooldown = time.Duration(v) * time.Minute
	}
	return pol
}

func ipKey(ip string) string     { return "ip:" + ip }
func userKey(user string) string { return "u:" + user }

func (l *loginLimiter) locked(ip, user string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	var remain time.Duration
	for _, k := range []string{ipKey(ip), userKey(user)} {
		if b := l.buckets[k]; b != nil && now.Before(b.lockedUntil) {
			if r := time.Until(b.lockedUntil); r > remain {
				remain = r
			}
		}
	}
	return remain > 0, remain
}

func (l *loginLimiter) fail(ip, user string, pol loginPolicy) bool {
	if !pol.enabled {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.sweepLocked(now, pol)
	newlyLocked := false
	if l.recordLocked(ipKey(ip), now, pol.window, pol.cooldown, pol.ipThreshold) {
		newlyLocked = true
	}
	if user != "" && l.recordLocked(userKey(user), now, pol.window, pol.cooldown, pol.userThreshold) {
		newlyLocked = true
	}
	return newlyLocked
}

func (l *loginLimiter) recordLocked(key string, now time.Time, window, cooldown time.Duration, threshold int) bool {
	b := l.buckets[key]
	if b == nil {
		b = &loginBucket{windowStart: now}
		l.buckets[key] = b
	}
	if now.Sub(b.windowStart) > window {
		b.windowStart = now
		b.count = 0
	}
	b.count++
	if b.count >= threshold {
		b.lockedUntil = now.Add(cooldown)
		b.count = 0
		b.windowStart = now
		return true
	}
	return false
}

func (l *loginLimiter) reset(ip, user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, ipKey(ip))
	delete(l.buckets, userKey(user))
}

func (l *loginLimiter) sweepLocked(now time.Time, pol loginPolicy) {
	if len(l.buckets) < 4096 {
		return
	}
	for k, b := range l.buckets {
		if now.After(b.lockedUntil) && now.Sub(b.windowStart) > pol.window {
			delete(l.buckets, k)
		}
	}
}
