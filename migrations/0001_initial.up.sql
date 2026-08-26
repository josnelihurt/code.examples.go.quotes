-- Mirrors the .NET 20260823175731_InitialCreate migration: the quotes table,
-- the unique fingerprint index, and the eight seed rows verbatim (ids 1-8,
-- fixed 2024-01-01T00:00:00Z timestamp so they sort first and tie-break
-- lexically). Columns are snake_case — both sides speak hand-written SQL, no
-- EF model must stay compatible (ADR 0007).
CREATE TABLE quotes (
    id varchar(64) PRIMARY KEY,
    text varchar(280) NOT NULL,
    author varchar(80) NOT NULL,
    normalized_fingerprint varchar(280) NOT NULL,
    created_at_utc timestamptz NOT NULL
);

CREATE UNIQUE INDEX quotes_normalized_fingerprint_key ON quotes (normalized_fingerprint);

INSERT INTO quotes (id, text, author, normalized_fingerprint, created_at_utc) VALUES
    ('1', 'Simplicity is the ultimate sophistication.', 'Leonardo da Vinci', 'simplicity is the ultimate sophistication', '2024-01-01T00:00:00Z'),
    ('2', 'Code is like humor. When you have to explain it, it''s bad.', 'Cory House', 'code is like humor when you have to explain it it s bad', '2024-01-01T00:00:00Z'),
    ('3', 'First, solve the problem. Then, write the code.', 'John Johnson', 'first solve the problem then write the code', '2024-01-01T00:00:00Z'),
    ('4', 'Experience is the name everyone gives to their mistakes.', 'Oscar Wilde', 'experience is the name everyone gives to their mistakes', '2024-01-01T00:00:00Z'),
    ('5', 'The only way to go fast is to go well.', 'Robert C. Martin', 'the only way to go fast is to go well', '2024-01-01T00:00:00Z'),
    ('6', 'Make it work, make it right, make it fast.', 'Kent Beck', 'make it work make it right make it fast', '2024-01-01T00:00:00Z'),
    ('7', 'Programs must be written for people to read.', 'Harold Abelson', 'programs must be written for people to read', '2024-01-01T00:00:00Z'),
    ('8', 'Talk is cheap. Show me the code.', 'Linus Torvalds', 'talk is cheap show me the code', '2024-01-01T00:00:00Z');
