-- +goose Up
ALTER TABLE questions
    ADD COLUMN is_derivable BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN is_verified BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN is_solvable BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN confidence_score NUMERIC (4,3),
    ADD COLUMN derivation_issue TEXT;


-- +goose Down
SELECT 'down SQL query';
ALTER TABLE questions
    DROP COLUMN is_derivable,
    DROP COLUMN is_verified,
    DROP COLUMN is_solvable,
    DROP COLUMN confidence_score,
    DROP COLUMN derivation_issue;