package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/polygon-zk-poker/server/internal/identity"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"
	"github.com/polygon-zk-poker/server/internal/engine"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"github.com/polygon-zk-poker/server/internal/store"
	"github.com/polygon-zk-poker/server/internal/table"
)

func TestProtocolHTTPAuthorizationAndAuthenticatedWSRecovery(t *testing.T) {
	serverKey, _ := crypto.GenerateKey()
	service := table.New(store.NewMemory(), serverKey, nil)
	now := uint64(time.Now().Unix())
	service.Now = func() time.Time { return time.Unix(int64(now), 0) }
	host := httptest.NewServer(WithProtocol(service))
	defer host.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	post := func(path string, v any, status int) []byte {
		t.Helper()
		body, _ := json.Marshal(v)
		r, e := client.Post(host.URL+path, "application/json", bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		raw, _ := io.ReadAll(r.Body)
		if r.StatusCode != status {
			t.Fatalf("%s: %d %s", path, r.StatusCode, raw)
		}
		return raw
	}
	spec := protocol.Spec{Domain: protocol.Domain{ChainID: 31337, Registry: common.HexToAddress("0x1234567890123456789012345678901234567890")}, TableID: protocol.SHA([]byte("ws-table")), HandID: protocol.SHA([]byte("ws-hand")), AuthIssuer: service.Address(), Config: engine.Config{Stacks: [9]int64{100, 100}, SmallBlind: 5, BigBlind: 10}, ValidUntil: now + 600, ActionLimit: 64}
	var own protocol.Session
	var ownSign func(common.Hash) string
	for i := 0; i < 2; i++ {
		if i > 0 {
			post("/api/v3/auth/logout", struct{}{}, 200)
		}
		guestRaw := post("/api/v3/auth/guest", struct{}{}, 200)
		var who identity.Principal
		_ = json.Unmarshal(guestRaw, &who)
		key, _ := crypto.GenerateKey()
		certRaw := post("/api/v3/sessions/issue", map[string]any{"domain": spec.Domain, "sessionKey": crypto.PubkeyToAddress(key.PublicKey), "publicKey": hexutil.Encode(crypto.FromECDSAPub(&key.PublicKey)), "tableId": spec.TableID, "nonce": "0", "expiresAt": strconv.FormatUint(now+3600, 10)}, 200)
		var s protocol.Session
		_ = json.Unmarshal(certRaw, &s)
		if s.Authorization.AccountID != who.ID {
			t.Fatal("identity certificate mismatch")
		}
		s.Signature, _ = protocol.Sign(key, s.ID)
		raw := post("/api/v3/sessions", s, 200)
		var saved protocol.Session
		if e := json.Unmarshal(raw, &saved); e != nil {
			t.Fatal(e)
		}
		if saved.ID != s.ID {
			t.Fatal("session mismatch")
		}
		spec.Players[i] = s.Authorization.PlayerID
		spec.Sessions[i] = s.ID
		if i == 0 {
			own = s
			ownSign = func(h common.Hash) string {
				sig, e := protocol.Sign(key, h)
				if e != nil {
					t.Fatal(e)
				}
				return sig
			}
		}
	}
	post("/api/v3/hands", spec, 200)
	req := table.ReadRequest{SessionID: own.ID, Timestamp: now}
	req.Signature = ownSign(table.ReadDigest(&table.Snapshot{Spec: spec}, req))
	raw := post("/api/v3/hands/"+spec.HandID.Hex()+"/recover", map[string]any{"sessionId": req.SessionID, "timestamp": strconv.FormatUint(req.Timestamp, 10), "signature": req.Signature, "after": "0", "knownReceipt": common.Hash{}}, 200)
	_ = raw
	wsURL := "ws" + strings.TrimPrefix(host.URL, "http") + "/api/v3/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	request := map[string]any{"type": "recover", "handId": spec.HandID, "read": req, "after": "0", "knownReceipt": common.Hash{}}
	if err = conn.WriteJSON(request); err != nil {
		t.Fatal(err)
	}
	var recovered table.Recovery
	if err = conn.ReadJSON(&recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.Public.Spec.HandID != spec.HandID || len(recovered.Events) != 1 || recovered.Hole != nil {
		t.Fatal("incorrect private recovery", recovered)
	}
	req.Signature = "0x" + strings.Repeat("00", 65)
	request["read"] = req
	_ = conn.WriteJSON(request)
	var rejected map[string]string
	if err = conn.ReadJSON(&rejected); err != nil || rejected["error"] == "" {
		t.Fatal("unauthenticated WS accepted", err)
	}
	headers := http.Header{"Origin": []string{"https://untrusted.example"}}
	bad, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if bad != nil {
		bad.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatal("WS origin accepted", err)
	}
	post("/api/v3/sessions", map[string]any{"unknown": true}, 400)
}
