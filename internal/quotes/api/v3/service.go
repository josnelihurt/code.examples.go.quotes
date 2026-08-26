// Package v3 is the quotes bounded context's v3 transport: the grpc-gateway
// runtime serving the annotated contract in internal/quotes/api/v3/contract,
// the QuoteService implementation over the layer-4 use cases, and the HTTP
// authentication middleware that owns the 401/403 wire shapes (ADR 0002).
// The proto stays the single contract of record — no hand-written routing
// exists; every wire semantic the .NET drift tests pin maps to a gateway knob
// pinned in NewGatewayMux.
package v3

import (
	"context"
	"errors"
	"math"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/api/v3/contract"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
)

// Service implements quotes.v3.QuoteService over the layer-4 use cases — the
// QuoteGrpcService port. Paging presence follows the `optional` contract: an
// unset query parameter defaults inside the service (1 / the domain's default
// page size), an explicit one passes through untouched so page=0 reaches the
// use case and answers the invalid-page rejection like every other version.
type Service struct {
	contract.UnimplementedQuoteServiceServer

	random  *application.GetRandomQuoteUseCase
	byID    *application.GetQuoteByIDUseCase
	list    *application.ListQuotesUseCase
	create  *application.CreateQuoteUseCase
	metrics *telemetry.Metrics
}

// NewService composes the transport over the use cases with the outcome
// counters (the .NET decorated-use-case analogue, recorded here because the
// grpc service is the single place every v3 call passes through).
func NewService(
	random *application.GetRandomQuoteUseCase,
	byID *application.GetQuoteByIDUseCase,
	list *application.ListQuotesUseCase,
	create *application.CreateQuoteUseCase,
	metrics *telemetry.Metrics,
) *Service {
	return &Service{random: random, byID: byID, list: list, create: create, metrics: metrics}
}

// GetRandomQuote returns a random quote from the catalog.
func (s *Service) GetRandomQuote(ctx context.Context, _ *contract.GetRandomQuoteRequest) (*contract.Quote, error) {
	quote, err := s.random.Execute(ctx)
	if err != nil {
		s.metrics.RecordQuotesRandom(ctx, outcomeFor(err))
		return nil, toStatusError(err)
	}
	s.metrics.RecordQuotesRandom(ctx, telemetry.OutcomeSuccess)
	return quoteMessage(quote), nil
}

// ListQuotes lists one page of the catalog in stable order.
func (s *Service) ListQuotes(ctx context.Context, req *contract.ListQuotesRequest) (*contract.ListQuotesResponse, error) {
	// Presence is the pointer: nil means the client sent nothing (defaults
	// apply — page 1, the domain's default page size), a non-nil zero is a sent
	// zero the use case rejects.
	query := application.ListQuotesQuery{Page: 1, PageSize: domain.DefaultPageSize}
	if req.Page != nil {
		query.Page = int(req.GetPage())
	}
	if req.PageSize != nil {
		query.PageSize = int(req.GetPageSize())
	}

	page, err := s.list.Execute(ctx, query)
	if err != nil {
		s.metrics.RecordQuotesList(ctx, outcomeFor(err))
		return nil, toStatusError(err)
	}
	s.metrics.RecordQuotesList(ctx, telemetry.OutcomeSuccess)
	return pageMessage(page), nil
}

// GetQuoteById returns one quote by id.
//
//nolint:revive // GetQuoteById is the rpc's name in the generated contract
func (s *Service) GetQuoteById(ctx context.Context, req *contract.GetQuoteByIdRequest) (*contract.Quote, error) {
	quote, err := s.byID.Execute(ctx, req.GetId())
	if err != nil {
		s.metrics.RecordQuotesGetByID(ctx, outcomeFor(err))
		return nil, toStatusError(err)
	}
	s.metrics.RecordQuotesGetByID(ctx, telemetry.OutcomeSuccess)
	return quoteMessage(quote), nil
}

// CreateQuote adds a quote to the catalog. The gateway answers 200 with the
// created quote — the HTTP rules have no way to express 201 + Location.
func (s *Service) CreateQuote(ctx context.Context, req *contract.CreateQuoteRequest) (*contract.Quote, error) {
	quote, err := s.create.Execute(ctx, application.CreateQuoteCommand{
		Text:   req.GetText(),
		Author: req.GetAuthor(),
	})
	if err != nil {
		s.metrics.RecordQuotesCreate(ctx, outcomeFor(err))
		return nil, toStatusError(err)
	}
	s.metrics.RecordQuotesCreate(ctx, telemetry.OutcomeSuccess)
	return quoteMessage(quote), nil
}

// toStatusError maps a use-case failure exactly like QuoteGrpcService's
// ToRpcException: the domain error's description travels as the status
// message byte-for-byte; the machine-readable code deliberately does not (the
// canonical carrier would be an ErrorInfo detail, which the .NET transcoding
// writer cannot render either). Everything that is not a domain failure is an
// infrastructure failure and answers Internal.
func toStatusError(err error) error {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case domain.NotFound().Code:
			return status.Error(codes.NotFound, domainErr.Description)
		case domain.DuplicateFingerprint().Code:
			return status.Error(codes.AlreadyExists, domainErr.Description)
		default:
			return status.Error(codes.InvalidArgument, domainErr.Description)
		}
	}
	return status.Error(codes.Internal, "An unexpected error occurred.")
}

// outcomeFor translates a failure into the ErrorOr outcome vocabulary the
// counters tag (success|not_found|error|conflict|invalid).
func outcomeFor(err error) string {
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		return telemetry.OutcomeError
	}
	switch domainErr.Code {
	case domain.NotFound().Code:
		return telemetry.OutcomeNotFound
	case domain.DuplicateFingerprint().Code:
		return telemetry.OutcomeConflict
	default:
		return telemetry.OutcomeInvalid
	}
}

// quoteMessage projects a transport DTO onto the contract message.
func quoteMessage(quote application.QuoteDto) *contract.Quote {
	return &contract.Quote{
		Id:     quote.ID,
		Text:   quote.Text,
		Author: quote.Author,
	}
}

// pageMessage projects a page DTO. The paging scalars are always set: they
// are `optional` in the contract precisely so the writer emits them on page
// one ("page":1) — the regression the declaration exists to prevent.
func pageMessage(page application.QuotePageDto) *contract.ListQuotesResponse {
	items := make([]*contract.Quote, 0, len(page.Items))
	for _, quote := range page.Items {
		items = append(items, quoteMessage(quote))
	}
	// G115: the paging scalars are bounded well inside int32 before they reach
	// here — ListQuotesUseCase rejects anything outside 1..domain.MaxPage and
	// 1..domain.MaxPageSize, and TotalPages is derived from those. TotalItems
	// is the catalog row count, so it is checked rather than asserted.
	return &contract.ListQuotesResponse{
		Items:      items,
		Page:       int32Ptr(int32(page.Page)),     //nolint:gosec // bounded by domain.MaxPage
		PageSize:   int32Ptr(int32(page.PageSize)), //nolint:gosec // bounded by domain.MaxPageSize
		TotalItems: int32Ptr(clampToInt32(page.TotalItems)),
		TotalPages: int32Ptr(int32(page.TotalPages)), //nolint:gosec // derived from the two bounds above
	}
}

func int32Ptr(v int32) *int32 { return &v }

// clampToInt32 saturates rather than wrapping. TotalItems is the catalog's row
// count, so unlike the paging scalars nothing upstream bounds it; a catalog
// that somehow exceeded int32 should report the largest number the contract
// can carry instead of a negative one.
func clampToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v)
}
