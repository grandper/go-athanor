package http

import (
	"net/http"

	"github.com/gorilla/mux"
)

// StaticHandler serves static files from a local directory under a URL prefix.
type StaticHandler struct {
	staticDir string
	urlPrefix string
}

// NewStaticHandler returns a StaticHandler serving the files of staticDir under urlPrefix.
func NewStaticHandler(staticDir string, urlPrefix string) *StaticHandler {
	return &StaticHandler{staticDir: staticDir, urlPrefix: urlPrefix}
}

// Register implements the Handler interface.
func (sh *StaticHandler) Register(router *mux.Router) error {
	fileServer := http.FileServer(http.Dir(sh.staticDir))
	router.PathPrefix(sh.urlPrefix).Handler(http.StripPrefix(sh.urlPrefix, fileServer))
	return nil
}

// StaticHandler implements the Handler interface.
var _ Handler = (*StaticHandler)(nil)
