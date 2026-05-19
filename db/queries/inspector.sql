-- name: GetUnverifiedQuestions :many
SELECT id, question_text, correct_choice
FROM questions
WHERE is_verified = false
ORDER BY id ASC;

-- name: UpdateQuestionVerification :exec
UPDATE questions
SET
    correct_choice = $2,
    explanation = $3,
    is_verified = true,
    is_solvable = $4,
    confidence_score = $5
WHERE id = $1;