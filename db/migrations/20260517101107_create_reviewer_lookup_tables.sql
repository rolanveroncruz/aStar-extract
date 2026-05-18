-- +goose Up
SELECT 'up SQL query';
CREATE TABLE subjects (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE -- e.g., 'Math', 'Science'
);

CREATE TABLE topics (
    id BIGSERIAL PRIMARY KEY,
    subject_id BIGINT NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    name TEXT NOT NULL UNIQUE -- e.g., 'Quadratic Equations'
);
CREATE INDEX idx_topics_subject ON topics(subject_id);

CREATE TABLE sources (
    id BIGSERIAL PRIMARY KEY,
    file_name TEXT NOT NULL UNIQUE, -- e.g., 'math_reviewer_v2.pdf'
    file_path TEXT NOT NULL,
    mother_folder text not null,
    file_size_bytes BIGINT NOT NULL,
    processed_start TIMESTAMP NOT NULL DEFAULT NOW(), --- start processing timestamp
    processed_end TIMESTAMP --- a timestamp here indicates we finished this document.

);
-- +goose Down
SELECT 'down SQL query';
DROP TABLE sources;
DROP TABLE topics;
DROP TABLE subjects;