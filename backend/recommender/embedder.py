from __future__ import annotations

from pathlib import Path

import numpy as np
import torch
from sentence_transformers import SentenceTransformer


class LocalSentenceEmbedder:
    def __init__(self, model_dir: Path, preferred_device: str = "cuda") -> None:
        device = preferred_device
        if preferred_device == "cuda" and not torch.cuda.is_available():
            device = "cpu"
        self.device = device
        self.model = SentenceTransformer(str(model_dir), device=device)

    def encode(self, texts: list[str], batch_size: int = 32) -> list[list[float]]:
        if not texts:
            return []
        vectors = self.model.encode(
            texts,
            batch_size=batch_size,
            convert_to_numpy=True,
            normalize_embeddings=True,
            show_progress_bar=False,
        )
        return np.asarray(vectors, dtype=np.float32).tolist()

