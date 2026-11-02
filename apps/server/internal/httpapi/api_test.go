package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestDemoAndVerify(t *testing.T) {
	h := New()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/demo", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var d Demo
	if e := json.Unmarshal(w.Body.Bytes(), &d); e != nil {
		t.Fatal(e)
	}
	if d.Hand.Street != "complete" || d.Report.Actions != 9 {
		t.Fatal(d)
	}
	raw, _ := json.Marshal(d.Audit)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/verify", bytes.NewReader(raw)))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	d.Audit.FinalStacks[0]++
	raw, _ = json.Marshal(d.Audit)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/verify", bytes.NewReader(raw)))
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
}
func TestRejectBodies(t *testing.T) {
	for _, v := range []struct{ path, body string }{{"/api/demo", "{}"}, {"/api/demo", strings.Repeat("x", 1025)}, {"/api/verify", "{"}, {"/api/verify", `{"unknown":1}`}, {"/api/verify", `{} {}`}} {
		w := httptest.NewRecorder()
		New().ServeHTTP(w, httptest.NewRequest("POST", v.path, strings.NewReader(v.body)))
		if w.Code != 400 {
			t.Fatal(w.Code, v)
		}
	}
}
func TestWebSocket(t *testing.T) {
	s := httptest.NewServer(New())
	defer s.Close()
	address := "ws" + strings.TrimPrefix(s.URL, "http") + "/ws"
	bad, resp, e := websocket.DefaultDialer.Dial(address, http.Header{"Origin": []string{"https://evil.invalid"}})
	if bad != nil {
		bad.Close()
	}
	if e == nil || resp.StatusCode != 403 {
		t.Fatal("cross origin accepted")
	}
	c, _, e := websocket.DefaultDialer.Dial(address, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	var hello map[string]string
	if e := c.ReadJSON(&hello); e != nil || hello["type"] != "ready" {
		t.Fatal(e, hello)
	}
	if e := c.WriteJSON(map[string]string{"type": "simulate"}); e != nil {
		t.Fatal(e)
	}
	var demo Demo
	if e := c.ReadJSON(&demo); e != nil || demo.Report.Actions != 9 {
		t.Fatal(e, demo)
	}
}
