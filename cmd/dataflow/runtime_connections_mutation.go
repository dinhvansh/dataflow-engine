package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type changeRuntimeConnection struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

type revokeRuntimeConnection struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

// Refuse a profile change while a previously bound stream has an active run.
// A metadata failure also blocks mutation. New runs are separately fenced by
// the immutable binding revision checked when the worker opens credentials.
func activeRuntimeProfile(ctx context.Context, tx pgx.Tx, id string) bool {
	var active bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connection_runtime_bindings b
	 JOIN managed_runs r ON r.stream_id=b.stream_id
	 WHERE (b.source_profile_id=$1::uuid OR b.target_profile_id=$1::uuid)
	 AND r.status IN ('queued','running'))`, id).Scan(&active)
	return err != nil || active
}

func (s *server) updateRuntimeProfile(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	var input changeRuntimeConnection
	if decodeRuntimeJSON(w, r, &input) != nil || uuid.Validate(input.ID) != nil || input.Revision < 1 {
		http.Error(w, "invalid profile revision", 400)
		return
	}
	tx, err := s.pg.Begin(ctx)
	if err != nil {
		http.Error(w, "registry unavailable", 503)
		return
	}
	defer tx.Rollback(ctx)
	var old runtimeConnection
	var ciphertext []byte
	err = tx.QueryRow(ctx, `SELECT id::text,name,driver,kind,host,port,database_name,username,status,revision,checked_at,secret_encrypted
	 FROM connection_runtime_profiles WHERE id=$1::uuid AND workspace='lab' FOR UPDATE`, input.ID).
		Scan(&old.ID, &old.Name, &old.Driver, &old.Kind, &old.Host, &old.Port, &old.Database, &old.Username,
			&old.Status, &old.Revision, &old.CheckedAt, &ciphertext)
	if err != nil {
		http.Error(w, "profile unavailable", 404)
		return
	}
	if old.Status == "revoked" || old.Revision != input.Revision {
		http.Error(w, "stale or revoked profile", 409)
		return
	}
	if activeRuntimeProfile(ctx, tx, input.ID) {
		http.Error(w, "profile has an active run or metadata is unavailable", 409)
		return
	}
	secret := input.Password
	if secret == "" {
		secret, err = decryptConnectionSecret(input.ID, ciphertext)
		if err != nil {
			http.Error(w, "credential unavailable", 503)
			return
		}
	}
	candidate := createRuntimeConnection{Name: input.Name, Driver: old.Driver, Host: input.Host, Port: input.Port,
		Database: input.Database, Username: input.Username, Password: secret}
	if !validRuntimeProfile(candidate) {
		http.Error(w, "invalid or unapproved QA connection", 422)
		return
	}
	if input.Password != "" {
		ciphertext, err = encryptConnectionSecret(input.ID, input.Password)
		if err != nil {
			http.Error(w, "credential store unavailable", 503)
			return
		}
	}
	var changed runtimeConnection
	err = tx.QueryRow(ctx, `UPDATE connection_runtime_profiles SET name=$2,host=$3,port=$4,
	 database_name=$5,username=$6,secret_encrypted=$7,status='draft',checked_at=NULL,
	 revision=revision+1 WHERE id=$1::uuid AND workspace='lab' AND revision=$8
	 RETURNING id::text,name,driver,kind,host,port,database_name,username,status,revision,checked_at`,
		input.ID, strings.TrimSpace(input.Name), input.Host, input.Port, input.Database, input.Username,
		ciphertext, input.Revision).
		Scan(&changed.ID, &changed.Name, &changed.Driver, &changed.Kind, &changed.Host, &changed.Port,
			&changed.Database, &changed.Username, &changed.Status, &changed.Revision, &changed.CheckedAt)
	if err != nil {
		http.Error(w, "profile update refused", 409)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		http.Error(w, "profile update unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(changed)
}

func (s *server) revokeRuntimeProfile(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	var input revokeRuntimeConnection
	if decodeRuntimeJSON(w, r, &input) != nil || uuid.Validate(input.ID) != nil || input.Revision < 1 {
		http.Error(w, "invalid profile revision", 400)
		return
	}
	tx, err := s.pg.Begin(ctx)
	if err != nil {
		http.Error(w, "registry unavailable", 503)
		return
	}
	defer tx.Rollback(ctx)
	var status string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT status,revision FROM connection_runtime_profiles
	 WHERE id=$1::uuid AND workspace='lab' FOR UPDATE`, input.ID).Scan(&status, &revision)
	if err != nil {
		http.Error(w, "profile unavailable", 404)
		return
	}
	if status == "revoked" || revision != input.Revision {
		http.Error(w, "stale or revoked profile", 409)
		return
	}
	if activeRuntimeProfile(ctx, tx, input.ID) {
		http.Error(w, "profile has an active run or metadata is unavailable", 409)
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE connection_runtime_profiles SET status='revoked',checked_at=NULL,
	 revision=revision+1 WHERE id=$1::uuid AND workspace='lab' AND revision=$2`, input.ID, input.Revision)
	if err != nil || tag.RowsAffected() != 1 {
		http.Error(w, "profile revocation unavailable", 503)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		http.Error(w, "profile revocation unavailable", 503)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
