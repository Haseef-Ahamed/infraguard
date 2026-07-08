"""
InfraGuard — Synthetic Infrastructure Telemetry Generator

Generates 30 days of realistic infrastructure telemetry data
for LSTM model training. Simulates patterns observed in real
cloud environments:
  - Business hours have higher change frequency
  - Post-deployment windows have elevated drift probability
  - Weekends have lower activity
  - Maintenance windows cause spikes
"""

import numpy as np
import pandas as pd
import psycopg2
from datetime import datetime, timedelta
import logging
import sys

logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
log = logging.getLogger(__name__)

# ── Constants ────────────────────────────────────────────────────────────────
DAYS            = 30
INTERVAL_MINS   = 5
POINTS_PER_DAY  = 24 * 60 // INTERVAL_MINS   # 288
TOTAL_POINTS    = DAYS * POINTS_PER_DAY       # 8640
TARGET_DRIFT_RATE = 0.15   # 15% positive class rate

# ── Feature engineering helpers ──────────────────────────────────────────────
def business_hour_factor(ts: datetime) -> float:
    """Returns activity multiplier based on time of day and day of week."""
    hour    = ts.hour
    weekday = ts.weekday()

    if weekday >= 5:                     # Weekend
        return 0.25
    if 9 <= hour <= 11:                  # Morning standup / sprint
        return 1.2
    if 14 <= hour <= 17:                 # Afternoon deployment window
        return 1.4
    if 18 <= hour <= 20:                 # EOD hotfixes
        return 0.9
    if 22 <= hour or hour <= 6:          # Night
        return 0.15
    return 0.7


def maintenance_window(ts: datetime) -> bool:
    """Returns True if timestamp falls in a weekly maintenance window."""
    return ts.weekday() == 2 and 2 <= ts.hour <= 4   # Wednesday 02:00–04:00


def generate_cluster(
    cluster_id: str,
    start: datetime,
    noise: float = 0.1,
    base_drift_rate: float = 0.12,
) -> pd.DataFrame:
    """
    Generates telemetry for one cluster over TOTAL_POINTS intervals.

    Features:
        cpu_utilization          — 0–100%
        memory_pressure          — 0–100%
        deploy_count_last_hour   — rolling deployment count
        change_frequency_last_day — rolling change count
        time_of_day_sin/cos      — cyclical encoding of hour
        day_of_week              — 0 (Monday) to 6 (Sunday)

    Label:
        drift_occurred           — 1 if drift happened in this interval
    """
    np.random.seed(abs(hash(cluster_id)) % (2**32))

    records      = []
    deploy_count = 0.0
    recent_chgs  = 0.0

    for i in range(TOTAL_POINTS):
        ts  = start + timedelta(minutes=INTERVAL_MINS * i)
        bhf = business_hour_factor(ts)
        mw  = maintenance_window(ts)

        # Simulate deployment events
        if np.random.random() < 0.015 * bhf:
            deploy_count += np.random.randint(1, 4)
        deploy_count = max(0.0, deploy_count - 0.08)

        # Change frequency — spikes after deployments and during maintenance
        base_cf    = 0.4 * bhf + deploy_count * 0.35 + (2.0 if mw else 0.0)
        recent_chgs = 0.88 * recent_chgs + np.random.poisson(max(0, base_cf))

        # CPU utilisation
        cpu = np.clip(
            30 * bhf
            + 18 * np.sin(i * 0.04)
            + deploy_count * 8
            + np.random.normal(0, noise * 22),
            0, 100,
        )

        # Memory pressure
        mem = np.clip(
            38 * bhf
            + 14 * np.sin(i * 0.025 + 1.2)
            + deploy_count * 5
            + np.random.normal(0, noise * 16),
            0, 100,
        )

        # Drift probability — higher when changes are frequent,
        # utilisation is elevated, or a deployment just happened
        drift_prob = np.clip(
            base_drift_rate
            + (recent_chgs / 25.0) * 0.38
            + (cpu / 100.0) * 0.18
            + (deploy_count / 6.0) * 0.28
            + (0.15 if mw else 0.0)
            + np.random.normal(0, 0.04),
            0.0, 1.0,
        )

        drift_occurred = int(np.random.random() < drift_prob * (TARGET_DRIFT_RATE / 0.20))

        records.append({
            "time":                      ts.isoformat(),
            "cluster_id":                cluster_id,
            "cpu_utilization":           round(float(cpu), 2),
            "memory_pressure":           round(float(mem), 2),
            "deploy_count_last_hour":    round(float(deploy_count), 2),
            "change_frequency_last_day": round(float(recent_chgs), 2),
            "time_of_day_sin":           round(np.sin(2 * np.pi * ts.hour / 24), 4),
            "time_of_day_cos":           round(np.cos(2 * np.pi * ts.hour / 24), 4),
            "day_of_week":               ts.weekday(),
            "drift_probability_label":   round(float(drift_prob), 4),
            "drift_occurred":            drift_occurred,
        })

    return pd.DataFrame(records)


def load_to_timescaledb(df: pd.DataFrame, host: str = "localhost", port: int = 5433) -> None:
    """Loads generated telemetry into TimescaleDB."""
    conn = psycopg2.connect(
        host=host, port=port,
        database="infraguard_ts",
        user="infraguard",
        password="infraguard_dev",
    )
    cur = conn.cursor()

    # Create hypertable if it doesn't exist
    cur.execute("""
        CREATE TABLE IF NOT EXISTS telemetry (
            time                      TIMESTAMPTZ NOT NULL,
            cluster_id                VARCHAR(50)  NOT NULL,
            cpu_utilization           NUMERIC(6,2),
            memory_pressure           NUMERIC(6,2),
            deploy_count_last_hour    NUMERIC(6,2),
            change_frequency_last_day NUMERIC(8,2),
            time_of_day_sin           NUMERIC(7,4),
            time_of_day_cos           NUMERIC(7,4),
            day_of_week               INTEGER,
            drift_probability_label   NUMERIC(6,4),
            drift_occurred            INTEGER
        );
    """)

    try:
        cur.execute(
            "SELECT create_hypertable('telemetry', 'time', if_not_exists => TRUE);"
        )
    except Exception:
        pass   # Already a hypertable

    conn.commit()

    # Bulk insert via COPY
    from io import StringIO
    output = StringIO()
    df.to_csv(output, index=False, header=False)
    output.seek(0)
    cur.copy_from(output, "telemetry", sep=",", columns=list(df.columns))
    conn.commit()
    cur.close()
    conn.close()
    log.info("Loaded %d rows into TimescaleDB", len(df))


def main(timescaledb_host: str = "localhost") -> pd.DataFrame:
    start = datetime(2026, 1, 1, 0, 0, 0)

    clusters = [
        ("cluster-prod-01",    0.08,  0.10),
        ("cluster-staging-01", 0.14,  0.18),
        ("cluster-dev-01",     0.20,  0.22),
    ]

    all_frames = []
    for cid, noise, drift_rate in clusters:
        log.info("Generating telemetry for %s ...", cid)
        df = generate_cluster(cid, start, noise=noise, base_drift_rate=drift_rate)
        all_frames.append(df)
        log.info(
            "  %s: %d records, drift rate=%.2f%%",
            cid, len(df), df["drift_occurred"].mean() * 100,
        )

    combined = pd.concat(all_frames, ignore_index=True)
    log.info("Total records: %d", len(combined))
    log.info("Overall drift rate: %.2f%%", combined["drift_occurred"].mean() * 100)

    # Save CSV for inspection
    csv_path = "/tmp/infraguard_telemetry.csv"
    combined.to_csv(csv_path, index=False)
    log.info("Saved CSV to %s", csv_path)

    # Load into TimescaleDB
    try:
        load_to_timescaledb(combined, host=timescaledb_host)
    except Exception as e:
        log.warning("Could not load to TimescaleDB: %s", e)
        log.info("Data saved to CSV only — run load_to_timescaledb() manually")

    return combined


if __name__ == "__main__":
    host = sys.argv[1] if len(sys.argv) > 1 else "localhost"
    main(timescaledb_host=host)

