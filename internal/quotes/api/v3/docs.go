package v3

import (
	"net/http"

	openapiembed "github.com/josnelihurt/code.examples.go.quotes/docs/openapi"
)

// OpenAPIPath and ScalarPath are the transport's documentation routes —
// unauthenticated, outside the gateway's route table (ADR 0002, item g).
const (
	OpenAPIPath = "/openapi/v3.json"
	ScalarPath  = "/scalar"
)

// serveOpenAPIDocument serves the embedded frozen document verbatim: what
// Scalar renders and what the contract-drift job diffs are the same bytes.
func serveOpenAPIDocument(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiembed.QuotesV3JSON)
}

// scalarPage adapts the docsify site's Scalar reference page (docs/scalar in
// the .NET kit) to this API's own route: same CDN widget, theme and layout,
// pointed at the document this host serves.
const scalarPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Aspire Quotes — Scalar API Reference</title>
  <style>
    html, body { margin: 0; height: 100%; }
  </style>
</head>
<body>
  <div id="app"></div>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  <script>
    Scalar.createApiReference('#app', {
      theme: 'purple',
      layout: 'modern',
      sources: [
        {
          title: 'Quotes API v3 (grpc-gateway)',
          url: '/openapi/v3.json',
          default: true
        }
      ]
    });
  </script>
</body>
</html>
`

// serveScalarPage serves the API reference page pointed at /openapi/v3.json.
func serveScalarPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(scalarPage))
}
