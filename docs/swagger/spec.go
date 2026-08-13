package swagger

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPIJSON []byte

// ServeSpec writes the OpenAPI 3.0.3 document (used as /swagger/doc.json).
func ServeSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(openAPIJSON)
}
