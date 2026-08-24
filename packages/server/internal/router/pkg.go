package router

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"server/internal/storage"
)

// splitPkg splits a request path into a package name and the rest.
// net/http puts an already-decoded string into r.URL.Path, so all
// spelling variants ("/@scope/foo", "/@scope%2Ffoo", "/%40scope%2Ffoo")
// converge to a single form. Router patterns like /{pkg}/-/{file} don't work
// here: the slash inside a scoped name breaks segment splitting.
func splitPkg(p string) (name, rest string) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", ""
	}
	if strings.HasPrefix(p, "@") {
		scope, rest, _ := strings.Cut(p, "/")
		name, tail, _ := strings.Cut(rest, "/")
		if name == "" {
			return "", ""
		}
		return scope + "/" + name, tail
	}
	name, rest, _ = strings.Cut(p, "/")
	if name == "" {
		return "", ""
	}
	return name, rest
}

// GET /@scope/foo, /@scope%2Ffoo, /%40scope%2Ffoo — package document
// GET /<name>/-/<file>.tgz — tarball download
func handlePkg(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, rest := splitPkg(r.URL.Path)
		if name == "" || (rest != "" && !strings.HasPrefix(rest, "-/")) {
			WriteError(w, ErrNotFound)
			return
		}
		if rest == "" {
			serveManifest(w, r, store, name)
			return
		}
		serveTarball(w, r, store, name, strings.TrimPrefix(rest, "-/"))
	}
}

func serveManifest(w http.ResponseWriter, r *http.Request, store storage.Storage, name string) {
	data, err := store.GetManifest(r.Context(), name)
	if errors.Is(err, storage.ErrNotExist) {
		WriteError(w, ErrNotFound)
		return
	}
	if err != nil {
		WriteError(w, ErrInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func serveTarball(w http.ResponseWriter, r *http.Request, store storage.Storage, name, filename string) {
	rc, size, err := store.GetTarball(r.Context(), name, filename)
	if errors.Is(err, storage.ErrNotExist) {
		WriteError(w, ErrNotFound)
		return
	}
	if err != nil {
		WriteError(w, ErrInternal)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}
