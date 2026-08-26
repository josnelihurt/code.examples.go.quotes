package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRandomQuoteUseCase(t *testing.T) {
	t.Run("returns a quote from the repository", func(t *testing.T) {
		sample := sampleQuote(t)
		repo := &stubQuoteRepository{random: sample}
		sut := application.NewGetRandomQuoteUseCase(repo)

		dto, err := sut.Execute(context.Background())

		require.NoError(t, err)
		assert.Equal(t, sample.ID, dto.ID)
		assert.Equal(t, sample.Text.Value, dto.Text)
		assert.Equal(t, sample.Author.Value, dto.Author)
		assert.Equal(t, 1, repo.getRandomCalls)
	})

	t.Run("returns not found when the catalog is empty", func(t *testing.T) {
		repo := &stubQuoteRepository{}
		sut := application.NewGetRandomQuoteUseCase(repo)

		_, err := sut.Execute(context.Background())

		requireCode(t, err, "quote.not_found")
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		repoErr := errors.New("catalog down")
		repo := &stubQuoteRepository{randomErr: repoErr}
		sut := application.NewGetRandomQuoteUseCase(repo)

		_, err := sut.Execute(context.Background())

		require.ErrorIs(t, err, repoErr)
	})
}

func TestGetRandomQuoteUseCaseHonorsCancellationBeforeLoadingAQuote(t *testing.T) {
	repo := &stubQuoteRepository{}
	sut := application.NewGetRandomQuoteUseCase(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := sut.Execute(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, repo.getRandomCalls)
}
