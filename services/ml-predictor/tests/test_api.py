"""
Tests for the InfraGuard ML Predictor FastAPI server.
Uses TestClient so no real server needs to be running.
"""

import sys, os, math, pickle, tempfile
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import numpy as np
import pytest
from unittest.mock import patch, MagicMock
from fastapi.testclient import TestClient


# ── Helpers ──────────────────────────────────────────────────────────────────
def make_telemetry_window(n=60, cpu=50.0, memory=45.0):
    """Creates a realistic telemetry window for testing."""
    return [
        {
            "cpu_utilization":           cpu,
            "memory_pressure":           memory,
            "deploy_count_last_hour":    1.5,
            "change_frequency_last_day": 5.0,
            "time_of_day_sin":           math.sin(2 * math.pi * 14 / 24),
            "time_of_day_cos":           math.cos(2 * math.pi * 14 / 24),
            "day_of_week":               1,
        }
        for _ in range(n)
    ]


def make_mock_model():
    """Creates a minimal mock GBM model that returns fixed probabilities."""
    mock = MagicMock()
    mock.predict_proba.return_value = np.array([[0.35, 0.65]])
    return mock


def make_model_artifacts(mock_model=None):
    """Creates model artifacts dict as saved by train.py."""
    return {
        "model":   mock_model or make_mock_model(),
        "seq_len": 60,
        "n_feat":  7,
    }


# ── Fixtures ─────────────────────────────────────────────────────────────────
@pytest.fixture
def client_with_model():
    """TestClient with a mock model loaded."""
    from src.main import app
    import src.main as main_module
    main_module.MODEL_ARTIFACTS = make_model_artifacts()
    return TestClient(app)


@pytest.fixture
def client_no_model():
    """TestClient with no model loaded."""
    from src.main import app
    import src.main as main_module
    main_module.MODEL_ARTIFACTS = {}
    return TestClient(app)


# ── Health endpoint tests ─────────────────────────────────────────────────────
class TestHealthEndpoints:

    def test_healthz_always_200(self, client_no_model):
        resp = client_no_model.get("/healthz")
        assert resp.status_code == 200
        assert resp.json()["status"] == "ok"

    def test_readyz_200_when_model_loaded(self, client_with_model):
        resp = client_with_model.get("/readyz")
        assert resp.status_code == 200
        assert resp.json()["model_loaded"] is True

    def test_readyz_503_when_no_model(self, client_no_model):
        resp = client_no_model.get("/readyz")
        assert resp.status_code == 503

    def test_model_info_200_when_loaded(self, client_with_model):
        resp = client_with_model.get("/model-info")
        assert resp.status_code == 200
        data = resp.json()
        assert "model_type" in data
        assert "features" in data
        assert "seq_len" in data

    def test_model_info_503_when_no_model(self, client_no_model):
        resp = client_no_model.get("/model-info")
        assert resp.status_code == 503


# ── Predict endpoint tests ────────────────────────────────────────────────────
class TestPredictEndpoint:

    def test_predict_returns_200(self, client_with_model):
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "cluster-prod-01",
            "telemetry_window": make_telemetry_window(60),
        })
        assert resp.status_code == 200

    def test_predict_response_schema(self, client_with_model):
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "cluster-prod-01",
            "telemetry_window": make_telemetry_window(60),
        })
        data = resp.json()
        assert "drift_probability" in data
        assert "anomaly" in data
        assert "anomaly_threshold" in data
        assert "forecast_windows" in data
        assert "cluster_id" in data
        assert data["cluster_id"] == "cluster-prod-01"

    def test_predict_probability_in_range(self, client_with_model):
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "test-cluster",
            "telemetry_window": make_telemetry_window(60),
        })
        data = resp.json()
        assert 0.0 <= data["drift_probability"] <= 1.0

    def test_predict_anomaly_true_when_high_prob(self, client_with_model):
        # Mock returns 0.65 probability — above 0.60 threshold
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "test-cluster",
            "telemetry_window": make_telemetry_window(60, cpu=90, memory=85),
        })
        data = resp.json()
        assert data["anomaly"] is True   # 0.65 >= 0.60

    def test_predict_forecast_windows_present(self, client_with_model):
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "test",
            "telemetry_window": make_telemetry_window(60),
        })
        fw = resp.json()["forecast_windows"]
        assert "minutes_15" in fw
        assert "minutes_30" in fw
        assert "minutes_60" in fw
        assert 0.0 <= fw["minutes_15"] <= 1.0
        assert 0.0 <= fw["minutes_30"] <= 1.0
        assert 0.0 <= fw["minutes_60"] <= 1.0

    def test_predict_too_few_points_returns_422(self, client_with_model):
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "test",
            "telemetry_window": make_telemetry_window(10),  # needs 60
        })
        assert resp.status_code == 422

    def test_predict_503_when_no_model(self, client_no_model):
        resp = client_no_model.post("/predict", json={
            "cluster_id":       "test",
            "telemetry_window": make_telemetry_window(60),
        })
        assert resp.status_code == 503

    def test_predict_invalid_cpu_returns_422(self, client_with_model):
        window = make_telemetry_window(60)
        window[0]["cpu_utilization"] = 150.0   # > 100 — invalid
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "test",
            "telemetry_window": window,
        })
        assert resp.status_code == 422

    def test_predict_accepts_exactly_60_points(self, client_with_model):
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "test",
            "telemetry_window": make_telemetry_window(60),
        })
        assert resp.status_code == 200

    def test_predict_accepts_more_than_60_points(self, client_with_model):
        resp = client_with_model.post("/predict", json={
            "cluster_id":       "test",
            "telemetry_window": make_telemetry_window(120),
        })
        assert resp.status_code == 200
