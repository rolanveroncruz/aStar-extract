-- +goose Up
SELECT 'up SQL query';

CREATE TABLE level_types( -- levels of difficulty e.g. easy, medium, hard
    id BIGSERIAL PRIMARY KEY,
    level_type TEXT NOT NULL UNIQUE
);

CREATE TABLE derived_questions (
    id BIGSERIAL PRIMARY KEY,
    original_question_id BIGINT REFERENCES  questions(id) ON DELETE SET NULL ,
    level_type BIGINT REFERENCES level_types(id),
    question_text TEXT NOT NULL,
    correct_choice TEXT NOT NULL,
    explanation TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_derived_questions_original ON derived_questions(original_question_id);

CREATE TABLE derived_choices (
    id BIGSERIAL PRIMARY KEY,
    question_id BIGINT NOT NULL REFERENCES derived_questions(id) ON DELETE CASCADE,
    choice_text TEXT NOT NULL,
    choice_letter TEXT NOT NULL
);
CREATE INDEX idx_derived_choices_question ON derived_choices(question_id);

-- +goose Down
SELECT 'down SQL query';
DROP TABLE derived_choices;
DROP TABLE derived_questions;
DROP TABLE level_types;
