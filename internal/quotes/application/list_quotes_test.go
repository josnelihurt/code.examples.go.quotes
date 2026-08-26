package application_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListQuotesUseCaseReturnsAPageWithThePagingArithmetic(t *testing.T) {
	sample := sampleQuote(t)
	repo := &stubQuoteRepository{
		page: domain.QuotePage{
			Items: []*domain.Quote{sample, sample, sample},
			Total: 8,
		},
	}
	sut := application.NewListQuotesUseCase(repo)

	page, err := sut.Execute(context.Background(), application.ListQuotesQuery{Page: 2, PageSize: 3})

	require.NoError(t, err)
	assert.Equal(t, 2, page.Page)
	assert.Equal(t, 3, page.PageSize)
	assert.Len(t, page.Items, 3)
	assert.Equal(t, 8, page.TotalItems)
	assert.Equal(t, 3, page.TotalPages)
	require.Len(t, repo.listCalls, 1)
	assert.Equal(t, stubListCall{skip: 3, take: 3}, repo.listCalls[0])
}

func TestListQuotesUseCaseRoundsTotalPagesUp(t *testing.T) {
	repo := &stubQuoteRepository{page: domain.QuotePage{Items: nil, Total: 7}}
	sut := application.NewListQuotesUseCase(repo)

	page, err := sut.Execute(context.Background(), application.ListQuotesQuery{Page: 3, PageSize: 3})

	require.NoError(t, err)
	assert.Equal(t, 3, page.TotalPages)
}

func TestListQuotesUseCaseTranslatesTheOneBasedPageIntoASkipOffset(t *testing.T) {
	repo := &stubQuoteRepository{}
	sut := application.NewListQuotesUseCase(repo)

	_, err := sut.Execute(context.Background(), application.ListQuotesQuery{Page: 4, PageSize: 25})

	require.NoError(t, err)
	require.Len(t, repo.listCalls, 1)
	assert.Equal(t, stubListCall{skip: 75, take: 25}, repo.listCalls[0])
}

func TestListQuotesUseCaseRejectsPagesOutsideTheAllowedRangeWithoutTouchingTheRepository(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
	}{
		{name: "page zero", page: 0, pageSize: 20},
		{name: "negative page", page: -1, pageSize: 20},
		{name: "page above the maximum", page: domain.MaxPage + 1, pageSize: 20},
		{name: "max int page", page: math.MaxInt, pageSize: 100},
		{name: "page size zero", page: 1, pageSize: 0},
		{name: "negative page size", page: 1, pageSize: -5},
		{name: "page size above the maximum", page: 1, pageSize: domain.MaxPageSize + 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &stubQuoteRepository{}
			sut := application.NewListQuotesUseCase(repo)

			_, err := sut.Execute(context.Background(), application.ListQuotesQuery{
				Page:     test.page,
				PageSize: test.pageSize,
			})

			requireCode(t, err, "quote.invalid_page_request")
			assert.Empty(t, repo.listCalls)
		})
	}
}

func TestListQuotesUseCaseAcceptsTheBounds(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
		want     stubListCall
	}{
		{
			name:     "maximum page",
			page:     domain.MaxPage,
			pageSize: domain.MaxPageSize,
			want:     stubListCall{skip: (domain.MaxPage - 1) * domain.MaxPageSize, take: domain.MaxPageSize},
		},
		{
			name:     "maximum page size",
			page:     1,
			pageSize: domain.MaxPageSize,
			want:     stubListCall{skip: 0, take: domain.MaxPageSize},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &stubQuoteRepository{}
			sut := application.NewListQuotesUseCase(repo)

			_, err := sut.Execute(context.Background(), application.ListQuotesQuery{
				Page:     test.page,
				PageSize: test.pageSize,
			})

			require.NoError(t, err)
			require.Len(t, repo.listCalls, 1)
			assert.Equal(t, test.want, repo.listCalls[0])
		})
	}
}

func TestListQuotesUseCaseReturnsEmptyItemsBeyondTheEndOfTheCatalog(t *testing.T) {
	repo := &stubQuoteRepository{page: domain.QuotePage{Items: nil, Total: 8}}
	sut := application.NewListQuotesUseCase(repo)

	page, err := sut.Execute(context.Background(), application.ListQuotesQuery{Page: 5, PageSize: 3})

	require.NoError(t, err)
	assert.Empty(t, page.Items)
	assert.Equal(t, 8, page.TotalItems)
	assert.Equal(t, 3, page.TotalPages)
}

func TestListQuotesUseCasePropagatesRepositoryErrors(t *testing.T) {
	repoErr := errors.New("catalog down")
	repo := &stubQuoteRepository{listErr: repoErr}
	sut := application.NewListQuotesUseCase(repo)

	_, err := sut.Execute(context.Background(), application.ListQuotesQuery{Page: 1, PageSize: 20})

	require.ErrorIs(t, err, repoErr)
}

func TestListQuotesUseCaseHonorsCancellationBeforeListing(t *testing.T) {
	repo := &stubQuoteRepository{}
	sut := application.NewListQuotesUseCase(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := sut.Execute(ctx, application.ListQuotesQuery{Page: 1, PageSize: 20})

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, repo.listCalls)
}
