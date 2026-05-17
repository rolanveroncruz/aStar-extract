-- +goose Up
SELECT 'up SQL query';
CREATE TABLE questions (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    subject_id BIGINT NOT NULL REFERENCES subjects(id),
    topic_id BIGINT REFERENCES topics(id), -- Optional, some questions might just have a subject
    references_diagram BOOLEAN NOT NULL DEFAULT FALSE, -- boolean to indicate if the question references a diagram
    question_text TEXT NOT NULL,
    correct_choice TEXT NOT NULL,
    explanation TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_questions_source ON questions(source_id);

CREATE TABLE choices (
    id BIGSERIAL PRIMARY KEY,
    question_id BIGINT NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    choice_text TEXT NOT NULL,
    choice_letter TEXT NOT NULL
);
CREATE INDEX idx_choices_question ON choices(question_id);
-- +goose Down
SELECT 'down SQL query';
DROP TABLE choices;
DROP TABLE questions;