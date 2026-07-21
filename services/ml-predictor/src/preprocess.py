"""
InfraGuard — Data Preprocessing Pipeline

Loads telemetry from TimescaleDB, normalises features,
and creates sliding-window sequences for LSTM training.
"""

import numpy as np
import pandas as pd
import psycopg2
import pickle
from sklearn.preprocessing import MinMaxScaler
from typing import Tuple, Dict
import logging

log = logging.getLogger(__name__)

FEATURES = [
    "cpu_utilization",
    "memory_pressure",
    "deploy_count_last_hour",
    "change_frequency_last_day",
    "time_of_day_sin",
    "time_of_day_cos",
    "day_of_week",
]
LABEL = "drift_occurred"
SEQ_LEN = 60  # 60 × 5 min = 5 hours of lookback
PRED_STEP = 6  # predict 30 minutes ahead (6 × 5 min)


def load_from_timescaledb(host: str = "localhost", port: int = 5433) -> pd.DataFrame:
    conn = psycopg2.connect(
        host=host,
        port=port,
        database="infraguard_ts",
        user="infraguard",
        password="infraguard_dev",
    )
    df = pd.read_sql(
        "SELECT * FROM telemetry ORDER BY cluster_id, time",
        conn,
        parse_dates=["time"],
    )
    conn.close()
    log.info("Loaded %d rows from TimescaleDB", len(df))
    return df


def load_from_csv(path: str = "/tmp/infraguard_telemetry.csv") -> pd.DataFrame:
    df = pd.read_csv(path, parse_dates=["time"])
    log.info("Loaded %d rows from CSV", len(df))
    return df


def create_sequences(
    df: pd.DataFrame,
    cluster_id: str,
) -> Tuple[np.ndarray, np.ndarray, MinMaxScaler]:
    # Return empty arrays if cluster not found
    if cluster_id not in df["cluster_id"].values:
        return np.array([]).reshape(0, SEQ_LEN, len(FEATURES)), np.array([]), MinMaxScaler()

    """
    Creates (SEQ_LEN, n_features) sliding windows for one cluster.
    Returns X, y, and the fitted scaler for this cluster.
    """
    cluster_df = df[df["cluster_id"] == cluster_id].sort_values("time").reset_index(drop=True)

    X_raw = cluster_df[FEATURES].values.astype(np.float32)
    y_raw = cluster_df[LABEL].values.astype(np.float32)

    scaler = MinMaxScaler(feature_range=(0, 1))
    X_scaled = scaler.fit_transform(X_raw)

    X_seqs, y_seqs = [], []
    max_i = len(X_scaled) - SEQ_LEN - PRED_STEP
    for i in range(max_i):
        X_seqs.append(X_scaled[i : i + SEQ_LEN])
        y_seqs.append(y_raw[i + SEQ_LEN + PRED_STEP])

    return np.array(X_seqs), np.array(y_seqs), scaler


def prepare_all_clusters(
    df: pd.DataFrame,
) -> Tuple[np.ndarray, np.ndarray, Dict[str, MinMaxScaler]]:
    """Prepares sequences for all clusters and returns combined arrays."""
    Xs, ys, scalers = [], [], {}

    for cid in df["cluster_id"].unique():
        X, y, scaler = create_sequences(df, cid)
        Xs.append(X)
        ys.append(y)
        scalers[cid] = scaler
        log.info(
            "Cluster %s: %d sequences, positive rate=%.2f%%",
            cid,
            len(X),
            y.mean() * 100,
        )

    X_all = np.vstack(Xs)
    y_all = np.concatenate(ys)
    log.info(
        "Combined: X=%s y=%s positive_rate=%.2f%%",
        X_all.shape,
        y_all.shape,
        y_all.mean() * 100,
    )
    return X_all, y_all, scalers


def save_artifacts(
    X: np.ndarray,
    y: np.ndarray,
    scalers: Dict[str, MinMaxScaler],
    prefix: str = "/tmp/infraguard",
) -> None:
    np.save(f"{prefix}_X.npy", X)
    np.save(f"{prefix}_y.npy", y)
    with open(f"{prefix}_scalers.pkl", "wb") as f:
        pickle.dump(scalers, f)
    log.info("Saved X, y, scalers to %s_*.{npy,pkl}", prefix)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
    try:
        df = load_from_timescaledb()
    except Exception as e:
        log.warning("TimescaleDB not available (%s), loading from CSV", e)
        df = load_from_csv()

    X, y, scalers = prepare_all_clusters(df)
    save_artifacts(X, y, scalers)
    log.info("Preprocessing complete")
