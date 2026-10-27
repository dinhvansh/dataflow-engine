package main

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

func qaConnectionName(name, prefix, id string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			prefix = "Connection"
		}
		if len([]rune(prefix)) > 40 {
			prefix = string([]rune(prefix)[:40])
		}
		name = prefix + " · " + id
	}
	if len([]rune(name)) < 1 || len([]rune(name)) > 80 {
		return "", errors.New("invalid QA connection name")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errors.New("invalid QA connection name")
		}
	}
	return name, nil
}

func insertQAConnection(ctx context.Context, tx pgx.Tx, id, name, sourceID, targetID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO qa_connections(id,name,source_profile_id,target_profile_id)
 VALUES($1::uuid,$2,$3::uuid,$4::uuid)`, id, name, sourceID, targetID)
	return err
}
