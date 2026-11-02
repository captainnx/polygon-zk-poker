package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polygon-zk-poker/server/internal/engine"
	"github.com/polygon-zk-poker/server/internal/fairness"
)

type Demo struct {
	Hand   *engine.Hand     `json:"hand"`
	Audit  *fairness.Audit  `json:"audit"`
	Report *fairness.Report `json:"report"`
}

func simulate() (*Demo, error) {
	a, h, e := fairness.SimulateNine()
	if e != nil {
		return nil, e
	}
	r, e := fairness.Verify(a)
	if e != nil {
		return nil, e
	}
	return &Demo{h, a, r}, nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func New() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"status": "ok", "mode": "local-demo", "phase1": true, "zkHandProof": false, "durableStorage": false, "sessionAuthentication": false})
	})
	mux.HandleFunc("POST /api/demo", func(w http.ResponseWriter, r *http.Request) {
		// No client-supplied game state is accepted by this simulator.
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		b, e := io.ReadAll(r.Body)
		if e != nil || len(b) != 0 {
			respond(w, 400, map[string]string{"error": "empty body required"})
			return
		}
		d, e := simulate()
		if e != nil {
			respond(w, 500, map[string]string{"error": "simulation failed"})
			return
		}
		respond(w, 200, d)
	})
	mux.HandleFunc("POST /api/verify", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var audit fairness.Audit
		if e := dec.Decode(&audit); e != nil {
			respond(w, 400, map[string]string{"error": "invalid audit JSON"})
			return
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			respond(w, 400, map[string]string{"error": "trailing data"})
			return
		}
		v, e := fairness.Verify(&audit)
		if e != nil {
			respond(w, 422, map[string]string{"error": e.Error()})
			return
		}
		respond(w, 200, v)
	})
	mux.HandleFunc("GET /ws", serveWS)
	return mux
}
func serveWS(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		v, e := url.Parse(origin)
		return e == nil && (v.Scheme == "http" || v.Scheme == "https") && v.Host == r.Host
	}}
	c, e := u.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(1024)
	_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if c.WriteJSON(map[string]string{"type": "ready", "mode": "local-demo"}) != nil {
		return
	}
	for {
		_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
		var req struct {
			Type string `json:"type"`
		}
		if c.ReadJSON(&req) != nil {
			return
		}
		_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if req.Type != "simulate" {
			if c.WriteJSON(map[string]string{"type": "error", "error": "unsupported demo command"}) != nil {
				return
			}
			continue
		}
		d, e := simulate()
		if e != nil {
			return
		}
		if c.WriteJSON(d) != nil {
			return
		}
	}
}
