-- name: GetItem :one
SELECT id, name, created_at
FROM items
WHERE id = $1;

-- name: ListItems :many
SELECT id, name, created_at
FROM items
ORDER BY created_at DESC
LIMIT $1;

-- name: CreateItem :one
INSERT INTO items (id, name, created_at)
VALUES ($1, $2, $3)
RETURNING id, name, created_at;
