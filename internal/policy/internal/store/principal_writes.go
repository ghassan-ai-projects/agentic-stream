package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// ReplaceGovernance makes the tenant's principals, role memberships and
// approval authorities match the document. Principals are never deleted:
// those the document omits are disabled, so past approvals keep their
// signers. Memberships and authorities of the tenant are replaced.
func (tx *Tx) ReplaceGovernance(ctx context.Context, document domain.PrincipalDocument, now string) (domain.PrincipalSummary, error) {
	if err := tx.disableTenantPrincipals(ctx, document.Tenant); err != nil {
		return domain.PrincipalSummary{}, err
	}
	if err := tx.upsertPrincipals(ctx, document, now); err != nil {
		return domain.PrincipalSummary{}, err
	}
	if err := tx.replaceRoles(ctx, document); err != nil {
		return domain.PrincipalSummary{}, err
	}
	return tx.GovernanceSummary(ctx, document.Tenant)
}

func (tx *Tx) upsertPrincipals(ctx context.Context, document domain.PrincipalDocument, now string) error {
	for _, principal := range document.Principals {
		key, err := principal.KeyBytes()
		if err != nil {
			return fmt.Errorf("principal key: %w", err)
		}
		if err := tx.upsertPrincipal(ctx, document.Tenant, principal, key, now); err != nil {
			return err
		}
	}
	return nil
}

// upsertPrincipal refuses to move a principal between tenants.
func (tx *Tx) upsertPrincipal(ctx context.Context, tenant string, principal domain.PrincipalEntry, key []byte, now string) error {
	result, err := tx.tx.ExecContext(ctx, `
		INSERT INTO principals (principal_id, tenant_id, status, public_key, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (principal_id) DO UPDATE SET status = excluded.status, public_key = excluded.public_key
		WHERE principals.tenant_id = excluded.tenant_id`,
		principal.ID, tenant, principal.EffectiveStatus(), key, now)
	if err != nil {
		return fmt.Errorf("write principal %s: %w", principal.ID, err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return fmt.Errorf("principal %s belongs to another tenant", principal.ID)
	}
	return nil
}

// disableTenantPrincipals disables every principal of the tenant; the
// upsert that follows restores the declared ones, so omitted principals stay
// disabled in the same transaction.
func (tx *Tx) disableTenantPrincipals(ctx context.Context, tenant string) error {
	if _, err := tx.tx.ExecContext(ctx, `UPDATE principals SET status = 'disabled' WHERE tenant_id = ?`, tenant); err != nil {
		return fmt.Errorf("disable omitted principals: %w", err)
	}
	return nil
}

// replaceRoles upserts the roles, then replaces the tenant's memberships and
// authorities with the document's.
func (tx *Tx) replaceRoles(ctx context.Context, document domain.PrincipalDocument) error {
	if err := tx.clearTenantGrants(ctx, document.Tenant); err != nil {
		return err
	}
	for _, role := range document.Roles {
		if err := tx.writeRole(ctx, document.Tenant, role); err != nil {
			return err
		}
	}
	return nil
}

func (tx *Tx) clearTenantGrants(ctx context.Context, tenant string) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM principal_roles WHERE principal_id IN (SELECT principal_id FROM principals WHERE tenant_id = ?)`, tenant); err != nil {
		return fmt.Errorf("clear role memberships: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM approval_authorities WHERE tenant_id = ?`, tenant); err != nil {
		return fmt.Errorf("clear approval authorities: %w", err)
	}
	return nil
}

func (tx *Tx) writeRole(ctx context.Context, tenant string, role domain.RoleEntry) error {
	if _, err := tx.tx.ExecContext(ctx, `INSERT INTO roles (role_id, role_name) VALUES (?, ?) ON CONFLICT (role_id) DO UPDATE SET role_name = excluded.role_name`, role.ID, role.Name); err != nil {
		return fmt.Errorf("write role %s: %w", role.ID, err)
	}
	for _, member := range role.Members {
		if _, err := tx.tx.ExecContext(ctx, `INSERT INTO principal_roles (principal_id, role_id) VALUES (?, ?)`, member, role.ID); err != nil {
			return fmt.Errorf("grant role %s to %s: %w", role.ID, member, err)
		}
	}
	return tx.writeAuthorities(ctx, tenant, role)
}

func (tx *Tx) writeAuthorities(ctx context.Context, tenant string, role domain.RoleEntry) error {
	for _, authority := range role.Authorities {
		for _, risk := range authority.Risks {
			if _, err := tx.tx.ExecContext(ctx, `INSERT INTO approval_authorities (tenant_id, entity_id, risk_class, role_id) VALUES (?, ?, ?, ?)`, tenant, authority.Entity, risk, role.ID); err != nil {
				return fmt.Errorf("grant %s authority on %s: %w", role.ID, authority.Entity, err)
			}
		}
	}
	return nil
}

// GovernanceSummary counts the tenant's stored governance.
func (tx *Tx) GovernanceSummary(ctx context.Context, tenant string) (domain.PrincipalSummary, error) {
	summary := domain.PrincipalSummary{Tenant: tenant}
	err := tx.tx.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM principals WHERE tenant_id = ? AND status = 'active'),
		(SELECT COUNT(*) FROM principals WHERE tenant_id = ? AND status = 'disabled'),
		(SELECT COUNT(DISTINCT pr.role_id) FROM principal_roles pr JOIN principals p ON p.principal_id = pr.principal_id WHERE p.tenant_id = ?),
		(SELECT COUNT(*) FROM principal_roles pr JOIN principals p ON p.principal_id = pr.principal_id WHERE p.tenant_id = ?),
		(SELECT COUNT(*) FROM approval_authorities WHERE tenant_id = ?)`,
		tenant, tenant, tenant, tenant, tenant).Scan(&summary.Active, &summary.Disabled, &summary.Roles, &summary.Memberships, &summary.Authorities)
	if err != nil {
		return domain.PrincipalSummary{}, fmt.Errorf("summarize governance: %w", err)
	}
	return summary, nil
}
