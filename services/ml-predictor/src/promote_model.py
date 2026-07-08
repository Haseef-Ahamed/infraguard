"""InfraGuard — Promote latest model to Production in local MLflow."""

import mlflow
from mlflow.tracking import MlflowClient
import os
import logging

logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
log = logging.getLogger(__name__)

ARTIFACT_DIR = os.path.expanduser("~/infraguard/services/ml-predictor/mlflow-artifacts")
MODEL_NAME   = "infraguard-drift-predictor"


def promote_latest_to_production():
    mlflow.set_tracking_uri(f"file://{ARTIFACT_DIR}/mlruns")
    client = MlflowClient()

    try:
        versions = client.get_latest_versions(MODEL_NAME)
    except Exception as e:
        log.error("Could not fetch model versions: %s", e)
        log.info("Run training first: python3 src/train.py")
        return

    if not versions:
        log.error("No versions found for model %s", MODEL_NAME)
        return

    latest = max(versions, key=lambda v: int(v.version))
    log.info("Latest version: %s (stage: %s)", latest.version, latest.current_stage)

    client.transition_model_version_stage(
        name=MODEL_NAME,
        version=latest.version,
        stage="Production",
        archive_existing_versions=True,
    )
    log.info("Model version %s promoted to Production ✓", latest.version)


if __name__ == "__main__":
    promote_latest_to_production()
