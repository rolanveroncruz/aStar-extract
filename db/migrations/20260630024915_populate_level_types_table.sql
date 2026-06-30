-- +goose Up
INSERT INTO level_types (id, level_type) VALUES
                                      (1, 'Easy'),
                                      (2, 'Medium'),
                                      (3, 'Hard')
ON CONFLICT (id) DO NOTHING;
-- +goose Down

DELETE FROM level_types where id in (1,2,3);