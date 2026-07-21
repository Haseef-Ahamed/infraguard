"""Tests for the preprocessing pipeline."""

import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import numpy as np
import pandas as pd
import pytest

from src.preprocess import (
    create_sequences,
    prepare_all_clusters,
    FEATURES,
    SEQ_LEN,
    PRED_STEP,
)


def make_fake_df(n: int = 300, cluster_id: str = "test-cluster") -> pd.DataFrame:
    """Creates a minimal fake telemetry DataFrame for testing."""
    np.random.seed(42)
    dates = pd.date_range("2026-01-01", periods=n, freq="5min")
    return pd.DataFrame({
        "time":                      dates,
        "cluster_id":                cluster_id,
        "cpu_utilization":           np.random.uniform(20, 80, n),
        "memory_pressure":           np.random.uniform(30, 70, n),
        "deploy_count_last_hour":    np.random.uniform(0, 3, n),
        "change_frequency_last_day": np.random.uniform(0, 10, n),
        "time_of_day_sin":           np.sin(np.linspace(0, 2 * np.pi, n)),
        "time_of_day_cos":           np.cos(np.linspace(0, 2 * np.pi, n)),
        "day_of_week":               [i % 7 for i in range(n)],
        "drift_probability_label":   np.random.uniform(0, 0.3, n),
        "drift_occurred":            (np.random.random(n) < 0.15).astype(int),
    })


class TestCreateSequences:

    def test_output_shapes(self):
        df = make_fake_df(300)
        X, y, scaler = create_sequences(df, "test-cluster")
        expected_seqs = 300 - SEQ_LEN - PRED_STEP
        assert X.shape == (expected_seqs, SEQ_LEN, len(FEATURES))
        assert y.shape == (expected_seqs,)

    def test_features_normalised_0_to_1(self):
        df = make_fake_df(300)
        X, y, scaler = create_sequences(df, "test-cluster")
        assert X.min() >= -0.01, "Features should be normalised to [0,1]"
        assert X.max() <=  1.01, "Features should be normalised to [0,1]"

    def test_scaler_not_none(self):
        df = make_fake_df(300)
        _, _, scaler = create_sequences(df, "test-cluster")
        assert scaler is not None

    def test_labels_are_binary(self):
        df = make_fake_df(300)
        _, y, _ = create_sequences(df, "test-cluster")
        unique = set(y.astype(int).tolist())
        assert unique.issubset({0, 1}), f"Labels must be 0 or 1, got {unique}"

    def test_empty_cluster_raises(self):
        df = make_fake_df(300, cluster_id="real-cluster")
        # Asking for a cluster that doesn't exist should return empty arrays
        X, y, scaler = create_sequences(df, "nonexistent-cluster")
        assert len(X) == 0


class TestPrepareAllClusters:

    def test_combines_multiple_clusters(self):
        df1 = make_fake_df(300, "cluster-a")
        df2 = make_fake_df(300, "cluster-b")
        combined = pd.concat([df1, df2], ignore_index=True)

        X, y, scalers = prepare_all_clusters(combined)
        assert "cluster-a" in scalers
        assert "cluster-b" in scalers
        assert len(X) > 0
        assert len(X) == len(y)

    def test_positive_rate_in_realistic_range(self):
        df1 = make_fake_df(500, "cluster-a")
        df2 = make_fake_df(500, "cluster-b")
        combined = pd.concat([df1, df2], ignore_index=True)

        _, y, _ = prepare_all_clusters(combined)
        positive_rate = y.mean()
        assert 0.02 <= positive_rate <= 0.60, (
            f"Positive rate {positive_rate:.2%} outside realistic range"
        )

    def test_x_shape_has_correct_features(self):
        df = make_fake_df(300, "cluster-x")
        X, y, _ = prepare_all_clusters(df)
        assert X.ndim == 3
        assert X.shape[1] == SEQ_LEN
        assert X.shape[2] == len(FEATURES)
