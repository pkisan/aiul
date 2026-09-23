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
