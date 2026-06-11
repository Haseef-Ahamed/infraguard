CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Every tracked cloud resource
CREATE TABLE IF NOT EXISTS resources (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  cloud         VARCHAR(10)  NOT NULL CHECK (cloud IN ('aws','azure','gcp')),
  region        VARCHAR(50)  NOT NULL,
  resource_type VARCHAR(100) NOT NULL,
  resource_id   VARCHAR(200) NOT NULL,
  current_state JSONB        NOT NULL DEFAULT '{}',
  iac_baseline  JSONB        NOT NULL DEFAULT '{}',
  last_checked  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  tags          JSONB        NOT NULL DEFAULT '{}',
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE(cloud, resource_id)
);
CREATE INDEX idx_resources_type ON resources(cloud, resource_type);

-- Point-in-time IaC baseline snapshots
CREATE TABLE IF NOT EXISTS state_snapshots (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  resource_id     VARCHAR(200) NOT NULL,
  state           JSONB        NOT NULL,
  snapshot_source VARCHAR(50)  NOT NULL DEFAULT 'terraform',
  git_commit_sha  VARCHAR(40),
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_snapshots_resource ON state_snapshots(resource_id, created_at DESC);

-- Core drift detection table
CREATE TABLE IF NOT EXISTS drift_events (
  id                    UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  resource_id           VARCHAR(200) NOT NULL,
  resource_type         VARCHAR(100) NOT NULL,
  cloud                 VARCHAR(10)  NOT NULL,
  region                VARCHAR(50)  NOT NULL DEFAULT 'us-east-1',
  change_type           VARCHAR(60)  NOT NULL,
  actor                 VARCHAR(200),
  previous_state        JSONB        NOT NULL DEFAULT '{}',
  new_state             JSONB        NOT NULL DEFAULT '{}',
  severity              VARCHAR(10)  NOT NULL DEFAULT 'INFO'
    CHECK (severity IN ('CRITICAL','HIGH','MEDIUM','LOW','INFO')),
  compliance_violations JSONB        NOT NULL DEFAULT '[]',
  drift_probability     NUMERIC(5,4),
  ml_anomaly            BOOLEAN      NOT NULL DEFAULT FALSE,
  status                VARCHAR(20)  NOT NULL DEFAULT 'OPEN'
    CHECK (status IN ('OPEN','PR_CREATED','REMEDIATED','SUPPRESSED')),
  pr_url                VARCHAR(500),
  pr_number             INTEGER,
  remediated_at         TIMESTAMPTZ,
  detected_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_drift_detected ON drift_events(detected_at DESC);
CREATE INDEX idx_drift_severity ON drift_events(severity, status);
CREATE INDEX idx_drift_resource ON drift_events(resource_id);

-- Immutable audit trail
CREATE TABLE IF NOT EXISTS audit_log (
  id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  action      VARCHAR(100) NOT NULL,
  actor       VARCHAR(200),
  drift_id    UUID         REFERENCES drift_events(id) ON DELETE SET NULL,
  resource_id VARCHAR(200),
  outcome     VARCHAR(20)  NOT NULL CHECK (outcome IN ('SUCCESS','FAILURE','SKIPPED')),
  details     JSONB        NOT NULL DEFAULT '{}',
  duration_ms INTEGER,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_created ON audit_log(created_at DESC);
CREATE INDEX idx_audit_drift   ON audit_log(drift_id);

-- Maps cloud principal IDs to human names
CREATE TABLE IF NOT EXISTS actors (
  id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  principal_id VARCHAR(500) NOT NULL,
  display_name VARCHAR(200),
  email        VARCHAR(200),
  cloud        VARCHAR(10)  NOT NULL,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE(principal_id)
);

-- Seed: LocalStack test user
INSERT INTO actors (principal_id, display_name, email, cloud)
VALUES (
  'arn:aws:iam::000000000000:root',
  'LocalStack Root (Dev)',
  'dev@infraguard.local',
  'aws'
) ON CONFLICT (principal_id) DO NOTHING;

SELECT 'Schema applied successfully' AS result;
