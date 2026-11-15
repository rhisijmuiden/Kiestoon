"""Speaks every prompt in prompts.tsv with Piper and writes <out>/<name>.raw.wav (22 kHz).

Usage: speak.py <model.onnx> <prompts.tsv> <out-dir>
"""
import os
import sys
import wave

from piper import PiperVoice, SynthesisConfig

model, prompts, out = sys.argv[1:4]
voice = PiperVoice.load(model)
config = SynthesisConfig(length_scale=1.05)
n = 0
with open(prompts, encoding="utf-8") as f:
    for line in f:
        line = line.rstrip("\n")
        if not line or line.startswith("#"):
            continue
        name, _, text = line.partition("\t")
        if not text.strip():
            sys.exit(f"no text for {name}")
        path = os.path.join(out, name + ".raw.wav")
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with wave.open(path, "wb") as wf:
            voice.synthesize_wav(text.strip(), wf, syn_config=config)
        n += 1
print(f"spoke {n} prompts")
