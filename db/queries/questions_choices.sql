-- name: ClearIncompleteQuestions :exec
DELETE FROM questions WHERE source_id = $1;

-- name: CreateInstructionContext :one
INSERT INTO instruction_contexts (source_id, context_text)
VALUES ($1, $2)
RETURNING id;

-- name: CreateQuestion :one
INSERT INTO questions (
    source_id, subject_id, topic_id, instruction_context_id, ref_no, references_diagram, question_text, correct_choice
) VALUES (
             $1, $2, $3, $4, $5, $6, $7, $8
         ) RETURNING id;

-- name: CreateChoice :exec
INSERT INTO choices (
    question_id, choice_text, choice_letter
) VALUES (
             $1, $2, $3
         );