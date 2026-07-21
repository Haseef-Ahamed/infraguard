"""
InfraGuard ML Predictor — FastAPI Inference Server
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

app = FastAPI(title="InfraGuard ML Predictor", version="1.0.0")

MODEL_ARTIFACTS: Dict[str, Any] = {}
MODEL_PATH = os.getenv("MODEL_PATH", "/tmp/infraguard_model.pkl")

FEATURES = [
    "cpu_utilization", "memory_pressure", "deploy_count_last_hour",
    "change_frequency_last_day", "time_of_day_sin", "time_of_day_cos", "day_of_week",
]
SEQ_LEN = 60


@app.on_event("startup")
async def load_model() -> None:
    global MODEL_ARTIFACTS
    if not os.path.exists(MODEL_PATH):
        log.warning("Model file not found at %s", MODEL_PATH)
        return
    with open(MODEL_PATH, "rb") as f:
        MODEL_ARTIFACTS = pickle.load(f)
    log.info("Model loaded from %s", MODEL_PATH)


class TelemetryPoint(BaseModel):
    cpu_utilization: float = Field(..., ge=0, le=100)
    memory_pressure: float = Field(..., ge=0, le=100)
    deploy_count_last_hour: float = Field(..., ge=0)
    change_frequency_last_day: float = Field(..., ge=0)
    time_of_day_sin: float = Field(..., ge=-1, le=1)
    time_of_day_cos: float = Field(..., ge=-1, le=1)
    day_of_week: int = Field(..., ge=0, le=6)


class PredictRequest(BaseModel):
    cluster_id: str
    telemetry_window: List[TelemetryPoint] = Field(..., min_length=SEQ_LEN)


class ForecastWindows(BaseModel):
    minutes_15: float
    minutes_30: float
    minutes_60: float


class PredictResponse(BaseModel):
    cluster_id: str
    drift_probability: float
    anomaly: bool
    anomaly_threshold: float
    forecast_windows: ForecastWindows
    model_type: str = "GradientBoostingClassifier"


def flatten_window(window):
    seq_len = MODEL_ARTIFACTS.get("seq_len", SEQ_LEN)
    n_feat = MODEL_ARTIFACTS.get("n_feat", len(FEATURES))
    points = window[-seq_len:]
    X_raw = np.array([[getattr(p, f) for f in FEATURES] for p in points], dtype=np.float32)
    X_flat = X_raw.reshape(1, seq_len * n_feat)
    X_mean = X_raw.mean(axis=0, keepdims=True)
    X_std = X_raw.std(axis=0, keepdims=True)
    X_max = X_raw.max(axis=0, keepdims=True)
    X_min = X_raw.min(axis=0, keepdims=True)
    return np.hstack([X_flat, X_mean, X_std, X_max, X_min])


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}


@app.get("/readyz")
async def readyz():
    if not MODEL_ARTIFACTS:
        raise HTTPException(status_code=503, detail="Model not loaded")
    return {"status": "ready", "model_loaded": True}


@app.get("/model-info")
async def model_info():
    if not MODEL_ARTIFACTS:
        raise HTTPException(status_code=503, detail="Model not loaded")
    return {
        "model_type": "GradientBoostingClassifier",
        "seq_len": MODEL_ARTIFACTS.get("seq_len", SEQ_LEN),
        "n_features": MODEL_ARTIFACTS.get("n_feat", len(FEATURES)),
        "features": FEATURES,
        "anomaly_threshold": 0.60,
    }


@app.post("/predict", response_model=PredictResponse)
async def predict(req: PredictRequest):
    if not MODEL_ARTIFACTS:
        raise HTTPException(status_code=503, detail="Model not loaded")
    model = MODEL_ARTIFACTS["model"]
    X = flatten_window(req.telemetry_window)
    prob = float(model.predict_proba(X)[0, 1])
    threshold = 0.60
    return PredictResponse(
        cluster_id=req.cluster_id,
        drift_probability=round(prob, 4),
        anomaly=prob >= threshold,
        anomaly_threshold=threshold,
        forecast_windows=ForecastWindows(
            minutes_15=round(min(1.0, prob * 0.75), 4),
            minutes_30=round(prob, 4),
            minutes_60=round(min(1.0, prob * 1.20), 4),
        ),
    )


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=int(os.getenv("PORT", "8001")))
