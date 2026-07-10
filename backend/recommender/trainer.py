from __future__ import annotations

import json
import time

from sklearn.linear_model import LogisticRegression
from sklearn.metrics import log_loss, roc_auc_score

from .features import build_training_examples, default_weights
from .storage import load_model_version, save_recommendation_model

MIN_DEPLOYABLE_AUC = 0.54


def train_ranking_model(conn, *, model_key: str, model_name: str, now_ms: int | None = None) -> dict[str, float]:
    now_ms = now_ms or int(time.time() * 1000)
    x_rows, y_rows, stats, feature_names = build_training_examples(conn, model_name, now_ms)
    exposure_count = int(stats.get("exposureCount", 0))
    click_count = int(stats.get("clickCount", 0))
    version = load_model_version(conn, model_key) + 1

    if exposure_count < 1000 or click_count < 100 or len(set(y_rows)) < 2:
        training_stats = dict(stats)
        training_stats.update({"fallback": True, "reason": "insufficient_samples", "trainedAt": now_ms})
        save_recommendation_model(
            conn,
            model_key=model_key,
            version=version,
            intercept=0.0,
            feature_json=json.dumps(default_weights(), ensure_ascii=False),
            training_stats=json.dumps(training_stats, ensure_ascii=False),
            trained_at=now_ms,
        )
        return training_stats

    clf = LogisticRegression(max_iter=1000, class_weight="balanced", solver="liblinear")
    clf.fit(x_rows, y_rows)
    probs = clf.predict_proba(x_rows)[:, 1]
    try:
        auc = float(roc_auc_score(y_rows, probs))
    except Exception:
        auc = 0.0
    try:
        loss = float(log_loss(y_rows, probs, labels=[0, 1]))
    except Exception:
        loss = 0.0

    if auc < MIN_DEPLOYABLE_AUC:
        training_stats = dict(stats)
        training_stats.update({
            "fallback": True,
            "reason": "low_auc",
            "auc": round(auc, 6),
            "logLoss": round(loss, 6),
            "trainedAt": now_ms,
            "sampleCount": len(y_rows),
        })
        save_recommendation_model(
            conn,
            model_key=model_key,
            version=version,
            intercept=0.0,
            feature_json=json.dumps(default_weights(), ensure_ascii=False),
            training_stats=json.dumps(training_stats, ensure_ascii=False),
            trained_at=now_ms,
        )
        return training_stats

    weights = {name: float(value) for name, value in zip(feature_names, clf.coef_[0])}
    training_stats = dict(stats)
    training_stats.update({
        "fallback": False,
        "auc": round(auc, 6),
        "logLoss": round(loss, 6),
        "trainedAt": now_ms,
        "sampleCount": len(y_rows),
    })
    save_recommendation_model(
        conn,
        model_key=model_key,
        version=version,
        intercept=float(clf.intercept_[0]),
        feature_json=json.dumps(weights, ensure_ascii=False),
        training_stats=json.dumps(training_stats, ensure_ascii=False),
        trained_at=now_ms,
    )
    return training_stats
