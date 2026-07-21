"""
InfraGuard ML Predictor — FastAPI Inference Server

Loads the trained GradientBoostingClassifier from disk and serves
drift probability predictions via REST API.

Endpoints:
    POST /predict   — returns drift_probability for a telemetry window
    GET  /healthz   — liveness probe
    GET  /readyz    — readiness probe (model must be loaded)
    GET  /model-info — model metadata
"""

import os
import pickle
import logging
import numpy as np
from typing import List, Dict, Any
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
import uvicorn

logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
log = logging.getLogger(__name__)

app = FastAPI(
    title="InfraGuard ML Predictor",
    description="Drift probability prediction service",
    version="1.0.0",
)

# ── Global model state ───────────────────────────────────────────────────────
MODEL_ARTIFACTS: Dict[str, Any] = {}
MODEL_PATH = os.getenv("MODEL_PATH", "/tmp/infraguard_model.pkl")

FEATURES = [
    "cpu_utilization",
    "memory_pressure",
    "deploy_count_last_hour",
    "change_frequency_last_day",
    "time_of_day_sin",
    "time_of_day_cos",
    "day_of_week",
]
SEQ_LEN = 60


# ── Startup — load model ─────────────────────────────────────────────────────
@app.on_event("startup")
async def load_model() -> None:
    global MODEL_ARTIFACTS
    if not os.path.exists(MODEL_PATH):
        log.warning("Model file not found at %s — server will start but /predict will fail", MODEL_PATH)
        return
    with open(MODEL_PATH, "rb") as f:
        MODEL_ARTIFACTS = pickle.load(f)
    log.info(
        "Model loaded from %s (seq_len=%d, n_feat=%d)",
        MODEL_PATH,
        MODEL_ARTIFACTS.get("seq_len", SEQ_LEN),
        MODEL_ARTIFACTS.get("n_feat", len(FEATURES)),
    )


# ── Request / Response schemas ───────────────────────────────────────────────
class TelemetryPoint(BaseModel):
    cpu_utilization:           float = Field(..., ge=0, le=100)
    memory_pressure:           float = Field(..., ge=0, le=100)
    deploy_count_last_hour:    float = Field(..., ge=0)
    change_frequency_last_day: float = Field(..., ge=0)
    time_of_day_sin:           float = Field(..., ge=-1, le=1)
    time_of_day_cos:           float = Field(..., ge=-1, le=1)
    day_of_week:               int   = Field(..., ge=0, le=6)


class PredictRequest(BaseModel):
    cluster_id:       str
    telemetry_window: List[TelemetryPoint] = Field(
        ..., min_length=SEQ_LEN,
        description=f"Must contain at least {SEQ_LEN} data points (5-minute intervals)",
    )


class ForecastWindows(BaseModel):
    minutes_15: float
    minutes_30: float
    minutes_60: float


class PredictResponse(BaseModel):
    cluster_id:        str
    drift_probability: float
    anomaly:           bool
    anomaly_threshold: float
    forecast_windows:  ForecastWindows
    model_type:        str = "GradientBoostingClassifier"


# ── Helper — flatten one window ──────────────────────────────────────────────
def flatten_window(window: List[TelemetryPoint]) -> np.ndarray:
    """
    Converts a list of TelemetryPoint into the same flattened feature
    vector that was used during training.
    """
    seq_len  = MODEL_ARTIFACTS.get("seq_len", SEQ_LEN)
    n_feat   = MODEL_ARTIFACTS.get("n_feat", len(FEATURES))

    # Take the last seq_len points
    points = window[-seq_len:]
    X_raw  = np.array(
        [[getattr(p, f) for f in FEATURES] for p in points],
        dtype=np.float32,
    )  # (seq_len, n_feat)

    # Replicate training flatten logic
    X_flat = X_raw.reshape(1, seq_len * n_feat)         # (1, seq_len*n_feat)
    X_mean = X_raw.mean(axis=0, keepdims=True)           # (1, n_feat)
    X_std  = X_raw.std(axis=0, keepdims=True)
    X_max  = X_raw.max(axis=0, keepdims=True)
    X_min  = X_raw.min(axis=0, keepdims=True)

    return np.hstack([X_flat, X_mean, X_std, X_max, X_min])  # (1, features)


# ── Endpoints ────────────────────────────────────────────────────────────────
@app.get("/healthz", tags=["Health"])
async def healthz() -> Dict[str, str]:
    return {"status": "ok"}


@app.get("/readyz", tags=["Health"])
async def readyz() -> Dict[str, Any]:
    if not MODEL_ARTIFACTS:
        raise HTTPException(status_code=503, detail="Model not loaded")
    return {"status": "ready", "model_loaded": True}


@app.get("/model-info", tags=["Model"])
async def model_info() -> Dict[str, Any]:
    if not MODEL_ARTIFACTS:
        raise HTTPException(status_code=503, detail="Model not loaded")
    return {
        "model_type": "GradientBoostingClassifier",
        "seq_len":    MODEL_ARTIFACTS.get("seq_len", SEQ_LEN),
        "n_features": MODEL_ARTIFACTS.get("n_feat", len(FEATURES)),
        "features":   FEATURES,
        "anomaly_threshold": 0.60,
    }


@app.post("/predict", response_model=PredictResponse, tags=["Prediction"])
async def predict(req: PredictRequest) -> PredictResponse:
    if not MODEL_ARTIFACTS:
        raise HTTPException(status_code=503, detail="Model not loaded")

    model = MODEL_ARTIFACTS["model"]

    # Build feature vector
    X = flatten_window(req.telemetry_window)

    # Get drift probability
    prob = float(model.predict_proba(X)[0, 1])

    # Simple forecast: adjust probability for different time horizons
    # In production this would use separate models per horizon
    prob_15m = round(min(1.0, prob * 0.75), 4)
    prob_30m = round(prob, 4)
    prob_60m = round(min(1.0, prob * 1.20), 4)

    threshold = 0.60

    return PredictResponse(
        cluster_id        = req.cluster_id,
        drift_probability = round(prob, 4),
        anomaly           = prob >= threshold,
        anomaly_threshold = threshold,
        forecast_windows  = ForecastWindows(
            minutes_15 = prob_15m,
            minutes_30 = prob_30m,
            minutes_60 = prob_60m,
        ),
    )


if __name__ == "__main__":
    uvicorn.run(
        "src.main:app",
        host="0.0.0.0",
        port=int(os.getenv("PORT", "8001")),
        reload=False,
        log_level="info",
    )
