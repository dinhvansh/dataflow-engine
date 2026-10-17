package connectionsecurity

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
)

// GrantAdmin is a server-only RBAC lifecycle primitive; no HTTP endpoint uses it.
// The caller actor must be derived from a VERIFIED identity, never user headers.
// MANAGE itself cannot be delegated by this service: only an independently
// authorized operator can bootstrap/manage trusted administrators.
type GrantAdmin struct {
	Catalog SecureCatalog
}

var actorKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func grantableScope(scope Scope) bool {
	return scope == View || scope == Use || scope == Test
}

// SetScope grants or revokes VIEW/USE/TEST in exactly one workspace.
// Replaying the same transition is an audited no-op (no duplicate event).
// The MANAGE grant remains row-locked through the authorization and write,
// so a concurrent revocation cannot sneak past the transaction boundary.
func (s GrantAdmin) SetScope(ctx context.Context, issuerActor, workspace, targetActor string, scope Scope, enable bool) (bool, error) {
	if !actorKeyPattern.MatchString(issuerActor) || !actorKeyPattern.MatchString(targetActor) ||
		!validWorkspace(workspace) || !grantableScope(scope) {
		return false, ErrForbidden
	}
	tx, err := s.Catalog.beginAuthorized(ctx, issuerActor, workspace, Manage)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var touched bool
	if enable {
		err = tx.QueryRow(ctx, `INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
			VALUES($1,$2,$3)
			ON CONFLICT (workspace_id,actor_id,scope)
			DO UPDATE SET granted_at=now(), revoked_at=NULL
			WHERE connection_security_grants.revoked_at IS NOT NULL
			RETURNING true`, workspace, targetActor, string(scope)).Scan(&touched)
	} else {
		err = tx.QueryRow(ctx, `UPDATE connection_security_grants SET revoked_at=now()
			WHERE workspace_id=$1 AND actor_id=$2 AND scope=$3 AND revoked_at IS NULL
			RETURNING true`, workspace, targetActor, string(scope)).Scan(&touched)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing changed. Authorization transaction still commits, to keep a
		// single grant snapshot; retrying is safe without duplicate audit rows.
		if err = tx.Commit(ctx); err != nil {
			return false, ErrUnavailable
		}
		return false, nil
	}
	if err != nil {
		return false, ErrUnavailable
	}
	action := "revoked"
	if enable {
		action = "granted"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO connection_security_grant_audit
		(workspace_id,actor_id,target_actor_id,scope,action)
		VALUES($1,$2,$3,$4,$5)`, workspace, issuerActor, targetActor, string(scope), action); err != nil {
		return false, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return false, ErrUnavailable
	}
	return true, nil
}

// AuthenticatedGrantAdmin verifies issuer and subject before requesting any DB
// operation. It must NEVER accept a browser-chosen subject as target without
// an independently trusted IdP directory/identity-lookup control plane.
type AuthenticatedGrantAdmin struct {
	Verifier *RS256Verifier
	Admin    GrantAdmin
}

func (s AuthenticatedGrantAdmin) SetScope(ctx context.Context, administratorJWT, workspace string, target VerifiedIdentity, scope Scope, enable bool) (bool, error) {
	admin, err := s.Verifier.Verify(administratorJWT)
	if err != nil || !target.Valid() || !admin.Valid() || admin.issuer != target.issuer {
		return false, ErrForbidden
	}
	return s.Admin.SetScope(ctx, admin.ActorID(), workspace, target.ActorID(), scope, enable)
}
