package parsers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// identity is who a tool's profile endpoint said is signed in.
type identity struct{ name, email string }

// identities remembers the last profile seen per host. The chat request itself
// rarely names the person — Cursor's token carries an opaque id, ChatGPT's only
// an email — but both apps fetch the signed-in profile when they start, over the
// same host, and that response has the name.
//
// ponytail: one entry per host, last one wins. Two people signed in to the same
// tool on one device at once would be attributed to whoever loaded last; key by
// token hash if that ever matters.
var (
	identitiesMu sync.Mutex
	identities   = map[string]identity{}
)

// IsIdentityEndpoint says whether NoteIdentity wants this exchange, so the proxy
// decompresses nothing it does not need.
func IsIdentityEndpoint(host, path string) bool {
	return (host == "chatgpt.com" && path == "/backend-api/me") ||
		(host == "api2.cursor.sh" && path == "/aiserver.v1.DashboardService/GetMe")
}

// NoteIdentity reads a profile response, if this exchange is one, and remembers
// who is signed in. Called for every decrypted exchange, parsed or not.
//
//	chatgpt.com  GET  /backend-api/me                          JSON {"name","email"}
//	api2.cursor.sh    /aiserver.v1.DashboardService/GetMe      proto 3=email 4=first 5=last
func NoteIdentity(ex Exchange) {
	if ex.Status != http.StatusOK {
		return
	}

	var id identity
	switch {
	case ex.Host == "chatgpt.com" && ex.Path == "/backend-api/me":
		var me struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		if json.Unmarshal(ex.RespBody, &me) != nil {
			return
		}
		id = identity{name: me.Name, email: me.Email}

	case ex.Host == "api2.cursor.sh" && ex.Path == "/aiserver.v1.DashboardService/GetMe":
		email, _ := pbString(ex.RespBody, []int{3})
		first, _ := pbString(ex.RespBody, []int{4})
		last, _ := pbString(ex.RespBody, []int{5})
		id = identity{name: strings.TrimSpace(first + " " + last), email: email}

	default:
		return
	}

	if id.name == "" && id.email == "" {
		return
	}

	identitiesMu.Lock()
	identities[ex.Host] = id
	identitiesMu.Unlock()
}

// Account names the AI account behind one exchange: the remembered profile for
// its host, else whatever the bearer token says.
//
// When the token names an email and the remembered profile is a DIFFERENT
// account (Codex signed in as one person, the browser as another), the token
// wins: it belongs to this very request.
func Account(ex Exchange) string {
	fromToken := AccountFromHeaders(ex.ReqHead)

	identitiesMu.Lock()
	id, known := identities[ex.Host]
	identitiesMu.Unlock()

	if !known || (fromToken != "" && !strings.EqualFold(fromToken, id.email) && fromToken != id.name) {
		return fromToken
	}
	return firstNonEmpty(id.name, id.email)
}

// AccountFromHeaders names the AI account a request was made with, when the
// credential itself says so. ChatGPT in the browser and Codex both send a JWT —
// a signed token whose middle part is plain JSON anyone can read — carrying the
// signed-in person's email. Only that claim is read; the token is never stored
// and its signature is not our business.
//
// Opaque tokens (Claude, Cursor's session keys) return "": those accounts are
// named by the tool's own profile endpoints, which need research first.
func AccountFromHeaders(h http.Header) string {
	token, found := strings.CutPrefix(h.Get("Authorization"), "Bearer ")
	if !found {
		return ""
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}

	var claims struct {
		Name   string `json:"name"`
		Email  string `json:"email"`
		OpenAI struct {
			Email string `json:"email"`
		} `json:"https://api.openai.com/profile"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}

	// A person's name reads best; the email is what OpenAI's tokens carry.
	return firstNonEmpty(claims.Name, claims.OpenAI.Email, claims.Email)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
