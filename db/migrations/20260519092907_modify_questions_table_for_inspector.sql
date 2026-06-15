-- +goose Up
ALTER TABLE questions
    ADD COLUMN is_derivable BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN derivation_issue TEXT;


-- +goose Down
SELECT 'down SQL query';
ALTER TABLE questions
    DROP COLUMN is_derivable,
    DROP COLUMN derivation_issue;