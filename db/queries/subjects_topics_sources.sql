-- name: UpsertSubject :one
INSERT INTO subjects (name) VALUES ($1)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id;

-- name: UpsertTopic :one
INSERT INTO topics (subject_id, name) VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id;

-- name: UpsertSource :one
INSERT INTO sources (file_name, file_path, mother_folder, file_size_bytes)
VALUES ($1, $2, $3,$4)
ON CONFLICT (file_name)
    DO UPDATE SET file_path = EXCLUDED.file_path,
                  mother_folder = EXCLUDED.mother_folder,
                  file_size_bytes = EXCLUDED.file_size_bytes
RETURNING id, file_name, file_path, mother_folder, file_size_bytes, processed_start, processed_end;

-- name: MarkSourceCompleted :exec
UPDATE sources SET processed_end = NOW() WHERE id = $1;
