package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polygon-zk-poker/server/internal/identity"
	"github.com/polygon-zk-poker/server/internal/store"
	"github.com/polygon-zk-poker/server/internal/table"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginCookiesAndSessionIssuanceRequireIdentity(t *testing.T) {
	key, _ := crypto.GenerateKey()
	s := table.New(store.NewMemory(), key, nil)
	server := httptest.NewServer(WithProtocol(s))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	send := func(path string, body any, expected int) *http.Response {
		t.Helper()
		raw, _ := json.Marshal(body)
		r, e := client.Post(server.URL+path, "application/json", bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		if r.StatusCode != expected {
			v, _ := io.ReadAll(r.Body)
			r.Body.Close()
			t.Fatalf("%s status %d %s", path, r.StatusCode, v)
		}
		return r
	}
	r := send("/api/v3/sessions/issue", map[string]any{}, 401)
	r.Body.Close()
	r = send("/api/v3/auth/guest", struct{}{}, 200)
	cookies := r.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].MaxAge <= 0 {
		t.Fatal("insecure cookie")
	}
	var guest identity.Principal
	_ = json.NewDecoder(r.Body).Decode(&guest)
	r.Body.Close()
	r = send("/api/v3/auth/register", map[string]string{"username": "http_test", "password": "test-only-long-password"}, 200)
	var account identity.Principal
	_ = json.NewDecoder(r.Body).Decode(&account)
	r.Body.Close()
	if account.ID != guest.ID || account.Kind != 1 {
		t.Fatal("upgrade changed player ID")
	}
	r = send("/api/v3/auth/logout", struct{}{}, 200)
	r.Body.Close()
	r = send("/api/v3/auth/login", map[string]string{"username": "http_test", "password": "wrong-password"}, 422)
	r.Body.Close()
	r = send("/api/v3/auth/login", map[string]string{"username": "http_test", "password": "test-only-long-password"}, 200)
	r.Body.Close()
	get, e := client.Get(server.URL + "/api/v3/auth/me")
	if e != nil || get.StatusCode != 200 {
		t.Fatal("login did not persist", e)
	}
	get.Body.Close()
	request, _ := http.NewRequest("POST", server.URL+"/api/v3/auth/logout", bytes.NewBufferString("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://other.example")
	bad, e := client.Do(request)
	if e != nil || bad.StatusCode != 403 {
		t.Fatal("cross-origin cookie mutation accepted", e)
	}
	bad.Body.Close()
	// Cookies can always be made Secure by deployment configuration.
	t.Setenv("COOKIE_SECURE", "true")
	rec := httptest.NewRecorder()
	writeLogin(rec, httptest.NewRequest("POST", "https://example.test", nil), identity.Login{Token: "test", Expires: uint64(time.Now().Unix()) + 60})
	if !rec.Result().Cookies()[0].Secure {
		t.Fatal("Secure cookie missing")
	}
}
func TestLoginRateLimitCannotBeBypassedWithForwardedHeader(t *testing.T) {
	rate := authRate{entries: map[string]authAttempt{}, now: time.Now}
	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "http://localhost/api/v3/auth/login", nil)
		r.Header.Set("X-Forwarded-For", "different-untrusted-ip")
		if !rate.allow(w, r) {
			t.Fatal("quota rejected early")
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://localhost/api/v3/auth/login", nil)
	r.Header.Set("X-Forwarded-For", "another-ip")
	if rate.allow(w, r) || w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("login brute-force limit bypassed")
	}
}
