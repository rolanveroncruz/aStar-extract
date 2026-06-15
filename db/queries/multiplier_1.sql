-- name: GetPendingQuestions :many
SELECT q.id, q.question_text
FROM questions q
         LEFT JOIN derived_questions dq ON q.id = dq.original_question_id
WHERE q.subject_id = $1
  AND q.is_derivable = true
  AND dq.id IS NULL
LIMIT 50;

-- name: GetChoicesForQuestion :many
SELECT choice_letter, choice_text
FROM choices
WHERE question_id = $1
ORDER BY choice_letter;

-- name: MarkQuestionUnderivable :exec
UPDATE questions
SET is_derivable = false, derivation_issue = $1
WHERE id = $2;

-- name: InsertDerivedQuestion :one
INSERT INTO derived_questions (
    original_question_id, subject_id, level_type, skills_tested, question_text, correct_choice, explanation
) VALUES (
             $1, $2, $3, $4, $5, $6, $7
         ) RETURNING id;

-- name: InsertDerivedChoice :exec
INSERT INTO derived_choices (
    question_id, choice_text, choice_letter
) VALUES (
             $1, $2, $3
         );