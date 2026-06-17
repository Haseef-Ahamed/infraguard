package state

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/infraguard/drift-engine/pkg/events"
)

// Store persists drift events and IaC baselines to PostgreSQL
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a connection pool to PostgreSQL
func NewStore(ctx context.Context, connStr string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("pgx: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pgx: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases all database connections
func (s *Store) Close() {
	s.pool.Close()
}

// SaveDriftEvent inserts a drift event into the drift_events table.
// Uses ON CONFLICT DO NOTHING so re-running detection on the same
// event (by ID) is idempotent.
func (s *Store) SaveDriftEvent(ctx context.Context, e *events.DriftEvent) error {
	prevJSON, err := json.Marshal(e.PreviousState)
	if err != nil {
		return fmt.Errorf("marshal previous_state: %w", err)
	}
	newJSON, err := json.Marshal(e.NewState)
	if err != nil {
		return fmt.Errorf("marshal new_state: %w", err)
	}
	violJSON, err := json.Marshal(e.Violations)
	if err != nil {
		return fmt.Errorf("marshal violations: %w", err)
	}

	severity := string(e.Severity)
	if severity == "" {
		severity = "INFO"
	}

	region := e.Region
	if region == "" {
		region = "us-east-1"
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO drift_events
			(id, resource_id, resource_type, cloud, region, change_type,
			 actor, previous_state, new_state, severity, compliance_violations, detected_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO NOTHING`,
		e.ID, e.ResourceID, e.ResourceType, e.Cloud, region, string(e.ChangeType),
		e.Actor, prevJSON, newJSON, severity, violJSON, e.DetectedAt,
	)
	if err != nil {
		return fmt.Errorf("insert drift_event: %w", err)
	}
	return nil
}

// UpsertBaseline stores the current IaC baseline snapshot for a resource.
// Called once during setup to seed baselines from the Phase 1 IaC apply.
func (s *Store) UpsertBaseline(ctx context.Context, resourceID string, state interface{}, source string) error {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO state_snapshots (resource_id, state, snapshot_source)
		VALUES ($1, $2, $3)`,
		resourceID, stateJSON, source,
	)
	if err != nil {
		return fmt.Errorf("insert state_snapshot: %w", err)
	}
	return nil
}

// GetLatestBaseline returns the most recent baseline snapshot for a resource.
// Returns nil, nil if no baseline exists yet.
func (s *Store) GetLatestBaseline(ctx context.Context, resourceID string) (map[string]interface{}, error) {
	var stateJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT state FROM state_snapshots
		 WHERE resource_id=$1
		 ORDER BY created_at DESC LIMIT 1`,
		resourceID,
	).Scan(&stateJSON)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("query baseline: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stateJSON, &result); err != nil {
		return nil, fmt.Errorf("unmarshal baseline: %w", err)
	}
	return result, nil
}

// CountOpenDriftEvents returns the number of unresolved drift events.
// Used by Prometheus metrics and the dashboard.
func (s *Store) CountOpenDriftEvents(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM drift_events WHERE status='OPEN'`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open drift events: %w", err)
	}
	return count, nil
}
