package httpapi

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polygon-zk-poker/server/internal/identity"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"github.com/polygon-zk-poker/server/internal/table"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const loginCookie = "poker_login_v3"

func loginToken(r *http.Request) string {
	c, e := r.Cookie(loginCookie)
	if e != nil {
		return ""
	}
	return c.Value
}
func writeLogin(w http.ResponseWriter, r *http.Request, v identity.Login) {
	http.SetCookie(w, &http.Cookie{Name: loginCookie, Value: v.Token, Path: "/api/v3", Expires: time.Unix(int64(v.Expires), 0), MaxAge: 7 * 86400, HttpOnly: true, Secure: r.TLS != nil || os.Getenv("COOKIE_SECURE") == "true", SameSite: http.SameSiteStrictMode})
	respond(w, 200, v.Principal)
}
func principal(w http.ResponseWriter, r *http.Request, a *identity.Service) (identity.Principal, bool) {
	p, e := a.Resolve(r.Context(), loginToken(r))
	if e != nil {
		respond(w, 401, map[string]string{"error": "login required"})
		return p, false
	}
	return p, true
}
func attachAuth(mux *http.ServeMux, a *identity.Service, s *table.Service) {
	limit := &authRate{entries: map[string]authAttempt{}, now: a.Now}
	mux.HandleFunc("POST /api/v3/auth/guest", func(w http.ResponseWriter, r *http.Request) {
		if !limit.allow(w, r) {
			return
		}
		var req struct{}
		if !decode(w, r, &req) {
			return
		}
		if p, e := a.Resolve(r.Context(), loginToken(r)); e == nil {
			respond(w, 200, p)
			return
		}
		v, e := a.Guest(r.Context())
		if e != nil {
			outcome(w, nil, e)
			return
		}
		writeLogin(w, r, v)
	})
	for _, mode := range []string{"login", "register"} {
		mux.HandleFunc("POST /api/v3/auth/"+mode, func(w http.ResponseWriter, r *http.Request) {
			if !limit.allow(w, r) {
				return
			}
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if !decode(w, r, &req) {
				return
			}
			var v identity.Login
			var e error
			if mode == "login" {
				v, e = a.Login(r.Context(), req.Username, req.Password)
			} else {
				p, err := a.Resolve(r.Context(), loginToken(r))
				var current *identity.Principal
				if err == nil {
					current = &p
				}
				v, e = a.Register(r.Context(), req.Username, req.Password, current)
			}
			if e != nil {
				outcome(w, nil, e)
				return
			}
			if old := loginToken(r); old != "" {
				_ = a.Logout(r.Context(), old)
			}
			writeLogin(w, r, v)
		})
	}
	mux.HandleFunc("GET /api/v3/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := principal(w, r, a); ok {
			respond(w, 200, p)
		}
	})
	mux.HandleFunc("POST /api/v3/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		var req struct{}
		if !decode(w, r, &req) {
			return
		}
		_ = a.Logout(r.Context(), loginToken(r))
		http.SetCookie(w, &http.Cookie{Name: loginCookie, Value: "", Path: "/api/v3", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil || os.Getenv("COOKIE_SECURE") == "true", SameSite: http.SameSiteStrictMode})
		respond(w, 200, map[string]bool{"loggedOut": true})
	})
	mux.HandleFunc("POST /api/v3/sessions/issue", func(w http.ResponseWriter, r *http.Request) {
		p, ok := principal(w, r, a)
		if !ok {
			return
		}
		var req struct {
			Domain     protocol.Domain `json:"domain"`
			SessionKey common.Address  `json:"sessionKey"`
			PublicKey  string          `json:"publicKey"`
			TableID    common.Hash     `json:"tableId"`
			Nonce      uint64          `json:"nonce,string"`
			ExpiresAt  uint64          `json:"expiresAt,string"`
		}
		if !decode(w, r, &req) {
			return
		}
		v, e := s.Issue(p.ID, p.Kind, req.Domain, req.SessionKey, req.PublicKey, req.TableID, req.Nonce, req.ExpiresAt)
		outcome(w, v, e)
	})
}
func protocolBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v3/") {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method == http.MethodPost {
				if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					respond(w, 415, map[string]string{"error": "application/json required"})
					return
				}
				if origin := r.Header.Get("Origin"); origin != "" {
					u, e := url.Parse(origin)
					if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != r.Host {
						respond(w, 403, map[string]string{"error": "cross-origin mutation denied"})
						return
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
func defaultTableConfig(s *table.Service) map[string]any {
	chain := uint64(31337)
	if n, e := strconv.ParseUint(os.Getenv("DEFAULT_CHAIN_ID"), 10, 64); e == nil {
		chain = n
	}
	registry := common.HexToAddress(os.Getenv("DEFAULT_REGISTRY_ADDRESS"))
	demo := registry == (common.Address{})
	if demo {
		registry = common.HexToAddress("0x1234567890123456789012345678901234567890")
	}
	return map[string]any{"domain": protocol.Domain{ChainID: chain, Registry: registry}, "tableId": protocol.SHA([]byte("ZKPOKER_DEFAULT_TABLE_V3")), "authIssuer": s.Address(), "demoOnly": demo, "note": fmt.Sprintf("server configured chain %d", chain)}
}
