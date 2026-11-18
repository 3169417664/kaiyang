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
	enabled         bool
	userThreshold   int
	ipThreshold     int
	globalThreshold int
	window          time.Duration
	cooldown        time.Duration
}

func (s *Server) loginPolicy() loginPolicy {
	pol := loginPolicy{
		enabled:         true,
		userThreshold:   10,
		ipThreshold:     30,
		globalThreshold: 300, // 分布式低速爆破的兜底；正常团队远达不到
		window:          15 * time.Minute,
		cooldown:        15 * time.Minute,
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
	if v := pg.GetInt("auth.lockout_global_threshold", 0); v > 0 {
		pol.globalThreshold = v
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
	if l == nil {
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	var remain time.Duration
	for _, k := range []string{ipKey(ip), userKey(user), "global"} {
		if b := l.buckets[k]; b != nil && now.Before(b.lockedUntil) {
			if r := time.Until(b.lockedUntil); r > remain {
				remain = r
			}
		}
	}
	return remain > 0, remain
}

// allow 是用于非登录端点（首启初始化等）的滑动窗口限流：只计数、不锁账号。
func (l *loginLimiter) allow(key string, limit int, window time.Duration) bool {
	if l == nil || limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[key]
	if b == nil || now.Sub(b.windowStart) > window {
		b = &loginBucket{windowStart: now}
		l.buckets[key] = b
	}
	b.count++
	return b.count <= limit
}

func (l *loginLimiter) fail(ip, user string, pol loginPolicy) bool {
	if l == nil || !pol.enabled {
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
	if pol.globalThreshold > 0 && l.recordLocked("global", now, pol.window, pol.cooldown, pol.globalThreshold) {
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
	if l == nil {
		return
	}
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
