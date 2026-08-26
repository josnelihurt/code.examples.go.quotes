// Package openapi embeds the frozen v3 quotes OpenAPI document so the
// API serves the exact bytes the contract-drift job diffs (ADR 0003): the
// file is build output generated from contracts/quotes/v3/quotes_v3.proto,
// committed under docs/openapi/, never hand-edited, and single-sourced — this
// package is the one reader the /openapi/v3.json endpoint has.
package openapi

import _ "embed"

// QuotesV3JSON is the frozen Swagger 2.0 document for the v3 transport.
//
//go:embed quotes-v3.openapi.json
var QuotesV3JSON []byte
