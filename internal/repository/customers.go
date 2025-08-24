package repository

import (
	"context"
	"database/sql"

	"github.com/jmehdipour/sms-gateway/internal/model"
	"github.com/jmoiron/sqlx"
)

type CustomersRepository interface {
	GetByAPIKey(ctx context.Context, apiKey string) (*model.Customer, error)
}

type CustomersRepositoryImpl struct {
	db *sqlx.DB
}

func NewCustomersRepository(db *sqlx.DB) *CustomersRepositoryImpl {
	return &CustomersRepositoryImpl{db: db}
}

var _ CustomersRepository = (*CustomersRepositoryImpl)(nil)

func (r *CustomersRepositoryImpl) GetByAPIKey(ctx context.Context, apiKey string) (*model.Customer, error) {
	var c model.Customer
	err := r.db.GetContext(ctx, &c, `
		SELECT id, name, api_key, status, rate_limit_rps, created_at, updated_at
		  FROM customers
		 WHERE api_key = ? LIMIT 1
	`, apiKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}
