package pgtracking

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
)

type Storage struct {
	db *pgxpool.Pool
}

func New(connString string) (*Storage, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, errors.Wrap(err, "parse pg config")
	}

	db, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, errors.Wrap(err, "connect pg")
	}

	s := &Storage{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) Close() {
	if s.db != nil {
		s.db.Close()
	}
}


