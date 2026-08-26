package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQuoteByIDUseCase(t *testing.T) {
	t.Run("returns the quote for a known id", func(t *testing.T) {
		sample := sampleQuote(t)
		repo := &stubQuoteRepository{byID: map[string]*domain.Quote{sample.ID: sample}}
		sut := application.NewGetQuoteByIDUseCase(repo)

		dto, err := sut.Execute(context.Background(), sample.ID)

		require.NoError(t, err)
		assert.Equal(t, sample.ID, dto.ID)
		assert.Equal(t, sample.Text.Value, dto.Text)
		assert.Equal(t, sample.Author.Value, dto.Author)
	})

	t.Run("returns not found for an unknown id", func(t *testing.T) {
		repo := &stubQuoteRepository{byID: map[string]*domain.Quote{}}
		sut := application.NewGetQuoteByIDUseCase(repo)

		_, err := sut.Execute(context.Background(), "missing")

		requireCode(t, err, "quote.not_found")
		assert.Equal(t, 1, repo.getByIDCalls)
	})

	blankIDs := []struct {
		name string
		id   string
	}{
		{name: "empty id", id: ""},
		{name: "blank id", id: "   "},
	}

	for _, test := range blankIDs {
		t.Run("returns not found for a "+test.name+" without touching the repository", func(t *testing.T) {
			repo := &stubQuoteRepository{byID: map[string]*domain.Quote{}}
			sut := application.NewGetQuoteByIDUseCase(repo)

			_, err := sut.Execute(context.Background(), test.id)

			requireCode(t, err, "quote.not_found")
			assert.Zero(t, repo.getByIDCalls)
		})
	}

	t.Run("propagates repository errors", func(t *testing.T) {
		repoErr := errors.New("catalog down")
		repo := &stubQuoteRepository{byIDErr: repoErr}
		sut := application.NewGetQuoteByIDUseCase(repo)

		_, err := sut.Execute(context.Background(), "7")

		require.ErrorIs(t, err, repoErr)
	})
}

func TestGetQuoteByIDUseCaseHonorsCancellationBeforeLoading(t *testing.T) {
	repo := &stubQuoteRepository{}
	sut := application.NewGetQuoteByIDUseCase(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := sut.Execute(ctx, "7")

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, repo.getByIDCalls)
}
