package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/websocket"
	"github.com/polygon-zk-poker/server/internal/identity"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"github.com/polygon-zk-poker/server/internal/store"
	"github.com/polygon-zk-poker/server/internal/table"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		respond(w, 400, map[string]string{"error": "invalid request"})
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		respond(w, 400, map[string]string{"error": "trailing data"})
		return false
	}
	return true
}
func outcome(w http.ResponseWriter, v any, e error) {
	if e != nil {
		status := 422
		if errors.Is(e, store.ErrNotFound) {
			status = 404
		}
		if errors.Is(e, store.ErrConflict) {
			status = 409
		}
		respond(w, status, map[string]string{"error": e.Error()})
		return
	}
	respond(w, 200, v)
}
func handID(w http.ResponseWriter, r *http.Request) (common.Hash, bool) {
	var id common.Hash
	if e := id.UnmarshalText([]byte(r.PathValue("id"))); e != nil {
		respond(w, 400, map[string]string{"error": "invalid hand ID"})
		return id, false
	}
	return id, true
}
func WithProtocol(service *table.Service, stores ...identity.Store) http.Handler {
	var authStore identity.Store = identity.NewMemory()
	if len(stores) > 0 {
		authStore = stores[0]
	}
	auth := identity.New(authStore)
	auth.Now = func() time.Time { return service.Now() }
	mux := http.NewServeMux()
	mux.Handle("/", New())
	attachAuth(mux, auth, service)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"status": "ok", "mode": "signed-protocol-v3", "durableStorage": isDurable(service), "sessionAuthentication": true, "proofGeneration": "external-native-prover", "productionReady": false})
	})
	mux.HandleFunc("GET /api/v3/hands/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := handID(w, r)
		if !ok {
			return
		}
		value, e := service.Public(r.Context(), id)
		outcome(w, value, e)
	})
	mux.HandleFunc("GET /api/v3/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := handID(w, r)
		if !ok {
			return
		}
		value, e := service.Store.Session(r.Context(), id)
		if e != nil {
			outcome(w, nil, e)
			return
		}
		respond(w, 200, map[string]any{"id": value.ID, "playerId": value.Authorization.PlayerID, "tableId": value.Authorization.TableID, "domain": value.Domain, "expiresAt": strconv.FormatUint(value.Authorization.ExpiresAt, 10), "revoked": value.Revoked, "session": value})
	})
	mux.HandleFunc("GET /api/v3/capabilities", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"version": "signed-hand-v3", "server": service.Address(), "actionCapacity": protocol.MaxActions, "durable": isDurable(service), "cache": service.Cache != nil, "sessionKeyScheme": "secp256k1", "sessionActionHash": "SHA256/raw-digest", "shuffle": "sha256-hash-sort-v3", "authIssuer": service.Address(), "defaultTable": defaultTableConfig(service)})
	})
	mux.HandleFunc("POST /api/v3/sessions", func(w http.ResponseWriter, r *http.Request) {
		p, ok := principal(w, r, auth)
		if !ok {
			return
		}
		var session protocol.Session
		if !decode(w, r, &session) {
			return
		}
		if session.Authorization.AccountID != p.ID {
			respond(w, 403, map[string]string{"error": "identity mismatch"})
			return
		}
		v, e := service.Authorize(r.Context(), session)
		outcome(w, v, e)
	})
	mux.HandleFunc("POST /api/v3/sessions/revoke", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID        common.Hash `json:"id"`
			Signature string      `json:"signature"`
		}
		if !decode(w, r, &req) {
			return
		}
		var e error
		if req.Signature != "" {
			e = service.Revoke(r.Context(), req.ID, req.Signature)
		} else {
			p, ok := principal(w, r, auth)
			if !ok {
				return
			}
			target, err := service.Store.Session(r.Context(), req.ID)
			if err != nil {
				e = err
			} else if target.Authorization.AccountID != p.ID {
				respond(w, 403, map[string]string{"error": "identity mismatch"})
				return
			} else {
				e = service.Store.Revoke(r.Context(), req.ID)
			}
		}
		outcome(w, map[string]bool{"revoked": e == nil}, e)
	})
	mux.HandleFunc("POST /api/v3/hands", func(w http.ResponseWriter, r *http.Request) {
		var spec protocol.Spec
		if !decode(w, r, &spec) {
			return
		}
		v, e := service.Create(r.Context(), spec)
		outcome(w, v, e)
	})
	mux.HandleFunc("POST /api/v3/hands/{id}/{kind}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := handID(w, r)
		if !ok {
			return
		}
		kind := r.PathValue("kind")
		switch kind {
		case "commit", "consent", "reveal":
			var req struct {
				Seat int `json:"seat"`
				protocol.SignedValue
			}
			if !decode(w, r, &req) {
				return
			}
			v, e := service.Value(r.Context(), id, req.Seat, kind, req.SignedValue)
			outcome(w, v, e)
		case "action":
			var req protocol.Move
			if !decode(w, r, &req) {
				return
			}
			v, e := service.Action(r.Context(), id, req)
			outcome(w, v, e)
		case "recover":
			var req struct {
				table.ReadRequest
				After uint64      `json:"after,string"`
				Known common.Hash `json:"knownReceipt"`
			}
			if !decode(w, r, &req) {
				return
			}
			v, e := service.Recover(r.Context(), id, req.ReadRequest, req.After, req.Known)
			outcome(w, v, e)
		case "audit":
			var req table.ReadRequest
			if !decode(w, r, &req) {
				return
			}
			v, e := service.Audit(r.Context(), id, req)
			outcome(w, v, e)
		case "timeout":
			var req struct{}
			if !decode(w, r, &req) {
				return
			}
			v, e := service.Timeout(r.Context(), id)
			outcome(w, v, e)
		default:
			respond(w, 404, map[string]string{"error": "unknown protocol command"})
		}
	})
	mux.HandleFunc("GET /api/v3/ws", func(w http.ResponseWriter, r *http.Request) { serveProtocolWS(service, w, r) })
	return protocolBoundary(mux)
}
func isDurable(s *table.Service) bool { _, ok := s.Store.(*store.Postgres); return ok }
func serveProtocolWS(s *table.Service, w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, CheckOrigin: func(r *http.Request) bool {
		if r.Header.Get("Origin") == "" {
			return true
		}
		o, e := url.Parse(r.Header.Get("Origin"))
		return e == nil && (o.Scheme == "http" || o.Scheme == "https") && o.Host == r.Host
	}}
	c, e := u.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(2 << 20)
	for {
		_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
		var req struct {
			Type  string            `json:"type"`
			ID    common.Hash       `json:"handId"`
			Read  table.ReadRequest `json:"read"`
			After string            `json:"after"`
			Known common.Hash       `json:"knownReceipt"`
			Move  protocol.Move     `json:"move"`
		}
		if c.ReadJSON(&req) != nil {
			return
		}
		var out any
		var err error
		switch req.Type {
		case "recover":
			after, e := strconv.ParseUint(req.After, 10, 64)
			if e != nil {
				err = e
			} else {
				out, err = s.Recover(r.Context(), req.ID, req.Read, after, req.Known)
			}
		case "action":
			out, err = s.Action(r.Context(), req.ID, req.Move)
		default:
			err = errors.New("unsupported WebSocket command")
		}
		if err != nil {
			out = map[string]string{"error": err.Error()}
		}
		_ = c.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if c.WriteJSON(out) != nil {
			return
		}
	}
}
