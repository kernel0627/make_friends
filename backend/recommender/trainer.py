from __future__ import annotations

import json
import time

from sklearn.linear_model import LogisticRegression
from sklearn.metrics import log_loss, roc_auc_score
from sklearn.model_selection import train_test_split

from .features import UNTRAINABLE_WEIGHTS, build_training_examples, default_weights
from .storage import load_model_version, save_recommendation_model

MIN_DEPLOYABLE_AUC = 0.54
HOLDOUT_FRACTION = 0.25
MIN_HOLDOUT_ROWS = 200


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

    # Score on held-out rows. Evaluating on the training set made the
    # MIN_DEPLOYABLE_AUC gate meaningless: an overfit model reports a high AUC
    # on data it already memorised and deploys regardless of real quality.
    holdout_used = False
    x_train, y_train, x_eval, y_eval = x_rows, y_rows, x_rows, y_rows
    if len(y_rows) >= MIN_HOLDOUT_ROWS:
        try:
            x_train, x_eval, y_train, y_eval = train_test_split(
                x_rows,
                y_rows,
                test_size=HOLDOUT_FRACTION,
                random_state=42,
                stratify=y_rows,
            )
            holdout_used = len(set(y_eval)) >= 2
            if not holdout_used:
                x_train, y_train, x_eval, y_eval = x_rows, y_rows, x_rows, y_rows
        except ValueError:
            # Too few of one class to stratify; fall back to full-set fitting.
            x_train, y_train, x_eval, y_eval = x_rows, y_rows, x_rows, y_rows

    clf = LogisticRegression(max_iter=1000, class_weight="balanced", solver="liblinear")
    clf.fit(x_train, y_train)
    probs = clf.predict_proba(x_eval)[:, 1]
    try:
        auc = float(roc_auc_score(y_eval, probs))
    except Exception:
        auc = 0.0
    try:
        loss = float(log_loss(y_eval, probs, labels=[0, 1]))
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

    # Carry the untrainable weights through: the scorer treats an absent
    # feature name as weight zero, so omitting them here would silently
    # disable those signals for every request served by this model.
    weights = {name: float(value) for name, value in zip(feature_names, clf.coef_[0])}
    weights.update(UNTRAINABLE_WEIGHTS)
    training_stats = dict(stats)
    training_stats.update({
        "fallback": False,
        "auc": round(auc, 6),
        "logLoss": round(loss, 6),
        "trainedAt": now_ms,
        "sampleCount": len(y_rows),
        "holdoutEval": holdout_used,
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
