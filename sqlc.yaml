version: "2"
sql:
  - engine: "postgresql"
    schema:
      - "db/schema.sql"
      - "migrations"
    queries: 
      - "internal/db/queries.sql"
    gen:
      go:
        package: "db"
        out: "internal/db/sqlc"
        sql_package: "pgx/v5"
