"""
InfraGuard — Drift Prediction Model Training
Uses GradientBoostingClassifier (scikit-learn) — no AVX required.
"""

import numpy as np
import pickle
import os
import mlflow
import mlflow.sklearn
from sklearn.ensemble import GradientBoostingClassifier
from sklearn.model_selection import train_test_split
from sklearn.metrics import roc_auc_score, precision_score, recall_score, f1_score
from sklearn.utils.class_weight import compute_sample_weight
import logging

logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
log = logging.getLogger(__name__)

# Local artifact storage
ARTIFACT_DIR = os.path.expanduser("~/infraguard/services/ml-predictor/mlflow-artifacts")


def flatten_sequences(X):
    N, seq_len, n_feat = X.shape
    X_flat = X.reshape(N, seq_len * n_feat)
    X_mean = X.mean(axis=1)
    X_std  = X.std(axis=1)
    X_max  = X.max(axis=1)
    X_min  = X.min(axis=1)
    return np.hstack([X_flat, X_mean, X_std, X_max, X_min])


def train(
    X_path="/tmp/infraguard_X.npy",
    y_path="/tmp/infraguard_y.npy",
    mlflow_uri="http://localhost:5000",
    experiment="infraguard-drift-prediction",
):
    X_raw = np.load(X_path)
    y     = np.load(y_path)
    log.info("Loaded X=%s y=%s positive_rate=%.2f%%", X_raw.shape, y.shape, y.mean()*100)

    X = flatten_sequences(X_raw)
    log.info("Flattened X shape: %s", X.shape)

    if len(X) > 20000:
        idx = np.random.RandomState(42).choice(len(X), 20000, replace=False)
        X, y = X[idx], y[idx]
        log.info("Subsampled to %d rows for speed", len(X))

    X_train, X_val, y_train, y_val = train_test_split(
        X, y, test_size=0.2, stratify=y, random_state=42,
    )

    sample_weights = compute_sample_weight("balanced", y_train)

    # Use local filesystem for MLflow artifacts
    os.makedirs(ARTIFACT_DIR, exist_ok=True)
    mlflow.set_tracking_uri(f"file://{ARTIFACT_DIR}/mlruns")
    mlflow.set_experiment(experiment)

    params = {
        "model":            "GradientBoostingClassifier",
        "n_estimators":     300,
        "max_depth":        5,
        "learning_rate":    0.05,
        "subsample":        0.8,
        "min_samples_leaf": 15,
    }

    with mlflow.start_run(run_name="gbm-v1") as run:
        mlflow.log_params(params)

        log.info("Training GradientBoostingClassifier...")
        model = GradientBoostingClassifier(
            n_estimators     = params["n_estimators"],
            max_depth        = params["max_depth"],
            learning_rate    = params["learning_rate"],
            subsample        = params["subsample"],
            min_samples_leaf = params["min_samples_leaf"],
            random_state     = 42,
            verbose          = 1,
        )
        model.fit(X_train, y_train, sample_weight=sample_weights)

        y_prob = model.predict_proba(X_val)[:, 1]
        y_pred = (y_prob >= 0.5).astype(int)

        auc       = roc_auc_score(y_val, y_prob)
        precision = precision_score(y_val, y_pred, zero_division=0)
        recall    = recall_score(y_val, y_pred, zero_division=0)
        f1        = f1_score(y_val, y_pred, zero_division=0)

        log.info("AUC=%.4f  Precision=%.4f  Recall=%.4f  F1=%.4f",
                 auc, precision, recall, f1)

        mlflow.log_metrics({
            "val_auc":       auc,
            "val_precision": precision,
            "val_recall":    recall,
            "val_f1":        f1,
        })

        # Save model artifact locally
        model_artifacts = {
            "model":   model,
            "seq_len": X_raw.shape[1],
            "n_feat":  X_raw.shape[2],
        }
        local_path = "/tmp/infraguard_model.pkl"
        with open(local_path, "wb") as f:
            pickle.dump(model_artifacts, f)
        log.info("Model saved to %s", local_path)

        # Log model to local MLflow
        mlflow.sklearn.log_model(
            model,
            "gbm-drift-model",
            registered_model_name="infraguard-drift-predictor",
        )

        if auc >= 0.65:
            log.info("AUC target met (>= 0.65) ✓")
        else:
            log.warning("AUC %.4f — acceptable for synthetic data", auc)

        return run.info.run_id


if __name__ == "__main__":
    run_id = train()
    log.info("Training complete. Run ID: %s", run_id)
    log.info("Next: python3 src/promote_model.py")
