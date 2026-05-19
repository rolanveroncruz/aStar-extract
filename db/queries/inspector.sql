-- name: GetUnverifiedQuestions :many
SELECT q.id,
       q.question_text,
        coalesce(
            json_agg(
                    json_build_object(
                            'letter', c.choice_letter,
                            'text', c.choice_text
                    ) order by c.choice_letter ASC
            )FILTER (where c.id IS NOT NULL),
        '[]'::json
        )::jsonb AS choices
FROM questions q
LEFT JOIN choices c ON q.id = c.question_id
WHERE q.is_verified = false
GROUP BY q.id
ORDER BY q.id ASC;

-- name: UpdateQuestionVerification :exec
UPDATE questions
SET
    correct_choice = $2,
    explanation = $3,
    is_verified = true,
    is_solvable = $4,
    confidence_score = $5
WHERE id = $1;