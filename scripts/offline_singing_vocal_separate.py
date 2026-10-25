#!/usr/bin/env python3
"""Separate vocals / accompaniment for offline singing drive."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import torch
import torchaudio
from demucs.apply import apply_model
from demucs.hdemucs import HDemucs
from demucs.pretrained import SOURCES


def remap_hdemucs_state(state: dict) -> dict:
    out = {}
    for key, value in state.items():
        nk = (
            key.replace("freq_encoder.", "encoder.")
            .replace("freq_decoder.", "decoder.")
            .replace("time_encoder.", "tencoder.")
            .replace("time_decoder.", "tdecoder.")
        )
        out[nk] = value
    return out


def load_hdemucs_high(checkpoint: Path) -> HDemucs:
    state = torch.load(checkpoint, map_location="cpu", weights_only=False)
    if not isinstance(state, dict):
        raise RuntimeError("checkpoint must be a state dict")
    model = HDemucs(
        sources=SOURCES,
        channels=48,
        depth=6,
        dconv_lstm=4,
        dconv_attn=4,
        hybrid=True,
    )
    remapped = remap_hdemucs_state(state)
    model.load_state_dict(remapped, strict=False)
    model.eval()
    return model


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--checkpoint", required=True, type=Path)
    parser.add_argument("--vocals-out", required=True, type=Path)
    parser.add_argument("--backing-out", required=True, type=Path)
    parser.add_argument(
        "--device",
        default="cuda" if torch.cuda.is_available() else "cpu",
    )
    args = parser.parse_args()

    if not args.input.is_file():
        print(f"input not found: {args.input}", file=sys.stderr)
        return 1
    if not args.checkpoint.is_file():
        print(f"checkpoint not found: {args.checkpoint}", file=sys.stderr)
        return 1

    wav, sr = torchaudio.load(str(args.input))
    if wav.dim() == 1:
        wav = wav.unsqueeze(0)
    if wav.shape[0] == 1:
        wav = wav.repeat(2, 1)

    model = load_hdemucs_high(args.checkpoint)
    device = torch.device(args.device)
    model.to(device)
    ref = wav.mean(0)
    wav = (wav - ref.mean()) / (ref.std() + 1e-8)
    sources = apply_model(
        model,
        wav[None].to(device),
        shifts=1,
        split=True,
        overlap=0.25,
        progress=False,
        device=device,
    )[0].cpu()

    vocals_idx = SOURCES.index("vocals")
    vocals = sources[vocals_idx] * ref.std() + ref.mean()
    backing = sum(sources[i] for i, name in enumerate(SOURCES) if name != "vocals")
    backing = backing * ref.std() + ref.mean()

    args.vocals_out.parent.mkdir(parents=True, exist_ok=True)
    args.backing_out.parent.mkdir(parents=True, exist_ok=True)
    torchaudio.save(str(args.vocals_out), vocals, sr)
    torchaudio.save(str(args.backing_out), backing, sr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
