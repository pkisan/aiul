package parsers

import (
	"encoding/base64"
	"net/http"
	"testing"
)

func bearer(claims string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiJ9."+base64.RawURLEncoding.EncodeToString([]byte(claims))+".c2ln")
	return h
}

func TestAccountFromHeaders(t *testing.T) {
	cases := map[string]struct {
		h    http.Header
		want string
	}{
		"openai profile claim": {bearer(`{"https://api.openai.com/profile":{"email":"alex@example.com"}}`), "alex@example.com"},
		"name wins":            {bearer(`{"name":"Alex John","email":"alex@example.com"}`), "Alex John"},
		"opaque token":         {http.Header{"Authorization": {"Bearer sk-ant-oat01-abc"}}, ""},
		"no header":            {http.Header{}, ""},
		"garbage payload":      {http.Header{"Authorization": {"Bearer a.!!!.c"}}, ""},
	}
	for name, c := range cases {
		if got := AccountFromHeaders(c.h); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// pbField2 encodes one length-delimited protobuf field, enough to build a
// synthetic GetMe reply without committing a real person's profile.
func pbField2(num int, s string) []byte {
	return append([]byte{byte(num<<3 | 2), byte(len(s))}, s...)
}

func TestIdentityFromProfileEndpoints(t *testing.T) {
	identities = map[string]identity{} // isolate from other tests

	NoteIdentity(Exchange{Host: "chatgpt.com", Path: "/backend-api/me", Status: 200,
		RespBody: []byte(`{"object":"user","email":"alex@example.com","name":"Alex John"}`)})

	var getMe []byte
	getMe = append(getMe, pbField2(1, "google-oauth2|user_x")...)
	getMe = append(getMe, pbField2(3, "sam@example.com")...)
	getMe = append(getMe, pbField2(4, "Sam")...)
	getMe = append(getMe, pbField2(5, "Lee")...)
	NoteIdentity(Exchange{Host: "api2.cursor.sh", Path: "/aiserver.v1.DashboardService/GetMe", Status: 200, RespBody: getMe})

	if got := Account(Exchange{Host: "chatgpt.com"}); got != "Alex John" {
		t.Errorf("chatgpt: got %q", got)
	}
	if got := Account(Exchange{Host: "api2.cursor.sh"}); got != "Sam Lee" {
		t.Errorf("cursor: got %q", got)
	}

	// The same account in the token: the profile's name is used.
	same := Exchange{Host: "chatgpt.com", ReqHead: bearer(`{"https://api.openai.com/profile":{"email":"alex@example.com"}}`)}
	if got := Account(same); got != "Alex John" {
		t.Errorf("same account: got %q", got)
	}

	// A different account in the token (Codex signed in as someone else): the
	// token belongs to this request, so it wins.
	other := Exchange{Host: "chatgpt.com", ReqHead: bearer(`{"https://api.openai.com/profile":{"email":"kim@example.com"}}`)}
	if got := Account(other); got != "kim@example.com" {
		t.Errorf("other account: got %q", got)
	}

	if got := Account(Exchange{Host: "claude.ai"}); got != "" {
		t.Errorf("unknown host: got %q", got)
	}
}
