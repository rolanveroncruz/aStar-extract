-- name: ClearIncompleteQuestions :exec
DELETE FROM questions WHERE source_id = $1;

-- name: CreateQuestion :one
INSERT INTO questions (
    source_id, subject_id, topic_id, references_diagram, question_text, correct_choice, explanation
) VALUES (
             $1, $2, $3, $4, $5, $6, $7
         ) RETURNING id;

-- name: CreateChoice :exec
INSERT INTO choices (
    question_id, choice_text, choice_letter
) VALUES (
             $1, $2, $3
         );