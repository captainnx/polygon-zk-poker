package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type authAttempt struct {
	start time.Time
	count int
}
type authRate struct {
	mu      sync.Mutex
	entries map[string]authAttempt
	now     func() time.Time
}

func (a *authRate) allow(w http.ResponseWriter, r *http.Request) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	ip, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		ip = r.RemoteAddr
	}
	now := a.now()
	v := a.entries[ip]
	if now.Sub(v.start) >= time.Minute {
		v = authAttempt{start: now}
	}
	if v.count >= 60 {
		w.Header().Set("Retry-After", "60")
		respond(w, 429, map[string]string{"error": "too many login attempts; retry later"})
		return false
	}
	if _, ok := a.entries[ip]; !ok && len(a.entries) >= 2048 {
		for k, v := range a.entries {
			if now.Sub(v.start) >= time.Minute {
				delete(a.entries, k)
			}
		}
		if len(a.entries) >= 2048 {
			respond(w, 429, map[string]string{"error": "login capacity reached"})
			return false
		}
	}
	v.count++
	a.entries[ip] = v
	return true
}
