package queries

import (
	"context"
	"time"

	"github.com/hwcopeland/iac/kai/internal/db"
)

// ----------------------------------------------------------------------------
// Team
// ----------------------------------------------------------------------------

// Team mirrors the teams table.
type Team struct {
	ID        string
	Name      string
	Slug      string
	CreatedBy string
	CreatedAt time.Time
}

// CreateTeam inserts a new team row and returns its UUID.
// slug must be globally unique; callers should validate before calling or
// handle the unique-constraint error (pgx pgerrcode.UniqueViolation).
func CreateTeam(ctx context.Context, pool *db.Pool, name, slug, createdBy string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO teams (name, slug, created_by)
		VALUES ($1, $2, $3)
		RETURNING id
	`, name, slug, createdBy).Scan(&id)
	return id, err
}

// GetTeam returns a single team by UUID.
// Returns pgx.ErrNoRows if not found.
func GetTeam(ctx context.Context, pool *db.Pool, teamID string) (*Team, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, name, slug, created_by, created_at
		FROM teams
		WHERE id = $1
	`, teamID)

	var t Team
	if err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedBy, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTeamsForUser returns all teams where the given user is a member,
// ordered newest first.
func ListTeamsForUser(ctx context.Context, pool *db.Pool, userID string) ([]Team, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.id, t.name, t.slug, t.created_by, t.created_at
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id
		WHERE tm.user_id = $1
		ORDER BY t.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

// AddTeamMember adds a user to a team with the given role ("owner" or "member").
// The role constraint is enforced at DB level by a CHECK on team_members.role.
// Returns an error on duplicate membership (pgx pgerrcode.UniqueViolation).
func AddTeamMember(ctx context.Context, pool *db.Pool, teamID, userID, role string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role)
		VALUES ($1, $2, $3)
	`, teamID, userID, role)
	return err
}

// IsTeamMember returns true if userID holds any role in teamID.
// Used by handler-layer authorization checks.
func IsTeamMember(ctx context.Context, pool *db.Pool, teamID, userID string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM team_members
			WHERE team_id = $1 AND user_id = $2
		)
	`, teamID, userID).Scan(&exists)
	return exists, err
}
