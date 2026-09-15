package item

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/chrome_skill/gen/db"
	"github.com/your-org/chrome_skill/internal/service/entity"
)

func toEntity(row db.Item) *entity.Item {
	return entity.RestoreItem(entity.ItemID(row.ID), row.Name, row.CreatedAt.Time)
}

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
