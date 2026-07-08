"""Tests for the synthetic data generator."""
import sys, os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
import pandas as pd
from datetime import datetime
from src.generate_data import generate_cluster, TOTAL_POINTS


class TestGenerateCluster:

    def test_output_length(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        assert len(df) == TOTAL_POINTS

    def test_required_columns_present(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        for col in ["time","cluster_id","cpu_utilization","memory_pressure",
                    "deploy_count_last_hour","change_frequency_last_day",
                    "time_of_day_sin","time_of_day_cos",
                    "day_of_week","drift_probability_label","drift_occurred"]:
            assert col in df.columns, f"Missing: {col}"

    def test_cpu_in_valid_range(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        assert df["cpu_utilization"].between(0, 100).all()

    def test_memory_in_valid_range(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        assert df["memory_pressure"].between(0, 100).all()

    def test_drift_label_is_binary(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        assert set(df["drift_occurred"].unique()).issubset({0, 1})

    def test_drift_rate_in_realistic_range(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        rate = df["drift_occurred"].mean()
        assert 0.02 <= rate <= 0.45, f"Drift rate {rate:.2%} outside range"

    def test_cluster_id_consistent(self):
        df = generate_cluster("my-cluster", datetime(2026, 1, 1))
        assert (df["cluster_id"] == "my-cluster").all()

    def test_time_series_monotonic(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        assert pd.to_datetime(df["time"]).is_monotonic_increasing

    def test_trig_features_in_range(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        assert df["time_of_day_sin"].between(-1.01, 1.01).all()
        assert df["time_of_day_cos"].between(-1.01, 1.01).all()

    def test_day_of_week_valid(self):
        df = generate_cluster("test-01", datetime(2026, 1, 1))
        assert df["day_of_week"].between(0, 6).all()
