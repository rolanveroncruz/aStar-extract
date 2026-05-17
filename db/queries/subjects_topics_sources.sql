-- name: UpsertSubject :one
INSERT INTO subjects (name) VALUES ($1)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id;

-- name: UpsertTopic :one
INSERT INTO topics (subject_id, name) VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id;

-- name: UpsertSource :one
INSERT INTO sources (file_name) VALUES ($1)
ON CONFLICT (file_name) DO UPDATE SET file_name = EXCLUDED.file_name
RETURNING id, processed_end;

-- name: MarkSourceCompleted :exec
UPDATE sources SET processed_end = NOW() WHERE id = $1;