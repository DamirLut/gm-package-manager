package router

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"server/internal/audit"
	"server/internal/auth"
)

const (
	loginBodyLimit  = 64 << 10
	loginNameLimit  = 100
	loginTailPrefix = "org.couchdb.user:"
)

type loginResponse struct {
	OK    string `json:"ok"`
	Token string `json:"token"`
}

// PUT /-/user/org.couchdb.user:<name> — npm adduser contract.
// Password comes from Authorization: Basic (preferred) or the JSON body.
func handleLogin(svc *auth.Service, auditor *audit.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store")

		name, ok := loginUserName(r.URL.Path)
		if !ok {
			WriteError(w, ErrBadRequest)
			return
		}
		pass, ok := loginPassword(w, r)
		if !ok {
			WriteError(w, ErrBadRequest)
			return
		}

		ip, ua := clientIP(r), r.UserAgent()
		tok, err := svc.Login(r.Context(), name, pass, ip)
		if err != nil {
			auditor.Record(audit.Event{
				Action: audit.ActionLoginFailed,
				Actor:  name,
				IP:     ip,
				UA:     ua,
			})
			w.Header().Set("WWW-Authenticate", "Bearer")
			WriteJSON(w, http.StatusUnauthorized, errorResponse{Error: "bad username/password, access denied"})
			return
		}

		auditor.Record(audit.Event{
			Action:      audit.ActionLoginSuccess,
			Actor:       name,
			TokenPrefix: tok.Prefix,
			IP:          ip,
			UA:          ua,
			Success:     true,
		})
		WriteJSON(w, http.StatusCreated, loginResponse{
			OK:    "you are authenticated as '" + name + "'",
			Token: tok.Secret,
		})
	}
}

// loginUserName parses the request tail: it must be exactly
// org.couchdb.user:<name>.
func loginUserName(path string) (string, bool) {
	tail := strings.TrimPrefix(path, "/-/user/")
	name, ok := strings.CutPrefix(tail, loginTailPrefix)
	if !ok || name == "" || len(name) > loginNameLimit {
		return "", false
	}
	return name, true
}

func loginPassword(w http.ResponseWriter, r *http.Request) (string, bool) {
	if _, pass, ok := r.BasicAuth(); ok && pass != "" {
		return pass, true
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, loginBodyLimit))
	if err != nil {
		return "", false
	}
	var body struct {
		Password string `json:"password"`
	}
	if json.Unmarshal(data, &body) != nil || body.Password == "" {
		return "", false
	}
	return body.Password, true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
