CREATE TABLE items (
    id         uuid PRIMARY KEY,
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX items_name_uniq ON items (name);

-- Список читается страницей по времени (ListItems: ORDER BY created_at DESC LIMIT),
-- без индекса это seq scan с сортировкой всей таблицы ради двух десятков строк.
CREATE INDEX items_created_at_idx ON items (created_at DESC);
