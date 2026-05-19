-- +goose Up
ALTER TABLE questions
    ADD COLUMN is_verified BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN is_solvable BOOLEAN,
    ADD COLUMN confidence_score NUMERIC(4,3);


-- +goose Down
SELECT 'down SQL query';
ALTER TABLE questions
    DROP COLUMN is_verified,
    DROP COLUMN is_solvable,
    DROP COLUMN confidence_score;