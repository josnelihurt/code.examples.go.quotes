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

func TestCreateQuoteUseCase(t *testing.T) {
	t.Run("creates and persists a valid quote", func(t *testing.T) {
		repo := &stubQuoteRepository{outcome: domain.QuoteAdded}
		sut := application.NewCreateQuoteUseCase(repo)

		dto, err := sut.Execute(context.Background(), application.CreateQuoteCommand{
			Text:   "Refactoring is the art of improving design.",
			Author: "Martin Fowler",
		})

		require.NoError(t, err)
		assert.Equal(t, "Refactoring is the art of improving design.", dto.Text)
		assert.Equal(t, "Martin Fowler", dto.Author)
		require.Len(t, repo.addCalls, 1)
		assert.Equal(t, "Refactoring is the art of improving design.", repo.addCalls[0].Text.Value)
		assert.Equal(t, "Martin Fowler", repo.addCalls[0].Author.Value)
		assert.Regexp(t, `^[0-9a-f]{32}$`, repo.addCalls[0].ID)
	})

	t.Run("returns conflict when the repository reports a duplicate fingerprint", func(t *testing.T) {
		repo := &stubQuoteRepository{outcome: domain.QuoteDuplicateFingerprint}
		sut := application.NewCreateQuoteUseCase(repo)

		_, err := sut.Execute(context.Background(), application.CreateQuoteCommand{
			Text:   "Talk is cheap. Show me the code!",
			Author: "Someone Else",
		})

		requireCode(t, err, "quote.duplicate_fingerprint")
	})

	t.Run("returns invalid without touching the repository", func(t *testing.T) {
		repo := &stubQuoteRepository{outcome: domain.QuoteAdded}
		sut := application.NewCreateQuoteUseCase(repo)

		_, err := sut.Execute(context.Background(), application.CreateQuoteCommand{
			Text:   "Nope.",
			Author: "X",
		})

		requireCode(t, err, "quote.text_too_short")
		assert.Empty(t, repo.addCalls)
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		repoErr := errors.New("catalog down")
		repo := &stubQuoteRepository{addErr: repoErr}
		sut := application.NewCreateQuoteUseCase(repo)

		_, err := sut.Execute(context.Background(), application.CreateQuoteCommand{
			Text:   "Talk is cheap. Show me the code.",
			Author: "Linus Torvalds",
		})

		require.ErrorIs(t, err, repoErr)
	})
}

func TestCreateQuoteUseCaseHonorsCancellationBeforeCreating(t *testing.T) {
	repo := &stubQuoteRepository{outcome: domain.QuoteAdded}
	sut := application.NewCreateQuoteUseCase(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := sut.Execute(ctx, application.CreateQuoteCommand{
		Text:   "Talk is cheap. Show me the code.",
		Author: "Linus Torvalds",
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, repo.addCalls)
}
