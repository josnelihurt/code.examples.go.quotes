package domain

// QuoteRules is the single source of truth for transport-level guards: outer
// layers size their request validation limits from these constants instead of
// duplicating magic numbers. The value-object bounds are re-exported here so
// consumers need not reach into the value objects themselves.
const (
	MinTextLength   = TextMinLength
	MaxTextLength   = TextMaxLength
	MinAuthorLength = AuthorMinLength
	MaxAuthorLength = AuthorMaxLength
	MinWordCount    = TextMinWordCount

	DefaultPageSize = 20
	MaxPageSize     = 100

	// MaxPage is the upper bound for the 1-based page number. The guard exists
	// so the 1-based → offset translation ((page - 1) * pageSize) can never
	// overflow int and turn a bad request into an unhandled failure.
	MaxPage = 10_000
)
