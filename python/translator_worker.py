#!/usr/bin/env python3
"""Line-delimited JSON worker for local translation models."""

import argparse
import json
import os
import re
import sys
import traceback

os.environ.setdefault("TOKENIZERS_PARALLELISM", "false")
os.environ.setdefault("TRANSFORMERS_VERBOSITY", "error")

# Redirected standard streams can inherit a legacy Windows code page. The Go
# parent and the line-delimited JSON protocol always use UTF-8.
for stream in (sys.stdin, sys.stdout, sys.stderr):
    if hasattr(stream, "reconfigure"):
        stream.reconfigure(encoding="utf-8")

NLLB_CODES = {
    "en": "eng_Latn",
    "zh": "zho_Hans",
    "ja": "jpn_Jpan",
    "ko": "kor_Hang",
    "fr": "fra_Latn",
    "de": "deu_Latn",
    "es": "spa_Latn",
    "ru": "rus_Cyrl",
}


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", required=True)
    parser.add_argument("--cache-dir", required=True)
    parser.add_argument("--download-only", action="store_true")
    return parser.parse_args()


def load_model(model_id, cache_dir):
    import torch
    from transformers import AutoModelForSeq2SeqLM, AutoTokenizer

    tokenizer = AutoTokenizer.from_pretrained(model_id, cache_dir=cache_dir)
    model = AutoModelForSeq2SeqLM.from_pretrained(model_id, cache_dir=cache_dir)
    device = "cuda" if torch.cuda.is_available() else "cpu"
    model.to(device)
    model.eval()
    return tokenizer, model, device


def content_token_ids(tokenizer, token_ids):
    """Return generated content without leading/trailing special tokens."""
    special_ids = set(tokenizer.all_special_ids)
    start = 0
    end = len(token_ids)
    while start < end and token_ids[start] in special_ids:
        start += 1
    while end > start and token_ids[end - 1] in special_ids:
        end -= 1
    return token_ids[start:end]


def collapse_degenerate_repetition(token_ids, source_token_count, min_repeats=3):
    """Collapse a repeated tail only when generation expanded abnormally.

    Small translation models can fall into a loop on very short inputs (for
    example, opus-mt-en-zh emits the same token five times for ``hello``).  A
    ratio guard keeps intentional repetition in the source from being changed.
    """
    if len(token_ids) < max(4, source_token_count * 4):
        return token_ids

    token_count = len(token_ids)
    max_period = token_count // min_repeats
    for period in range(1, max_period + 1):
        repeated_unit = token_ids[token_count - period :]
        repeat_start = token_count - period
        repeats = 1
        while repeat_start >= period and token_ids[repeat_start - period : repeat_start] == repeated_unit:
            repeat_start -= period
            repeats += 1
        if repeats >= min_repeats:
            return token_ids[:repeat_start] + repeated_unit
    return token_ids


def translate_one(tokenizer, model, device, text, generation):
    import torch

    encoded = tokenizer(
        text,
        return_tensors="pt",
        truncation=True,
        max_length=512,
    )
    source_token_count = max(
        1,
        sum(token_id not in tokenizer.all_special_ids for token_id in encoded["input_ids"][0].tolist()),
    )
    generation = {
        **generation,
        # Translation length normally tracks source length. Keeping a modest
        # floor still allows short sentences while avoiding hundreds of looped
        # tokens from a one-word input.
        "max_new_tokens": min(512, max(32, source_token_count * 4 + 16)),
    }
    encoded = {key: value.to(device) for key, value in encoded.items()}
    with torch.inference_mode():
        generated = model.generate(**encoded, **generation)
    generated_ids = content_token_ids(tokenizer, generated[0].tolist())
    generated_ids = collapse_degenerate_repetition(generated_ids, source_token_count)
    return tokenizer.decode(generated_ids, skip_special_tokens=True)


def translate(tokenizer, model, device, model_id, text, source, target):
    generation = {"num_beams": 4, "early_stopping": True}
    if "nllb" in model_id.lower():
        source_code = NLLB_CODES.get(source)
        target_code = NLLB_CODES.get(target)
        if not source_code or not target_code:
            raise ValueError("当前 NLLB 界面语言映射不受支持")
        tokenizer.src_lang = source_code
        generation["forced_bos_token_id"] = tokenizer.convert_tokens_to_ids(target_code)

    # Keep paragraphs and split long paragraphs below the models' 512-token limit.
    translated_parts = []
    for part in re.split(r"(\n+)", text):
        if not part:
            continue
        if part.startswith("\n"):
            translated_parts.append(part)
            continue
        token_ids = tokenizer(part, add_special_tokens=False)["input_ids"]
        chunks = [token_ids[i : i + 420] for i in range(0, len(token_ids), 420)]
        translated_chunks = []
        for chunk in chunks:
            chunk_text = tokenizer.decode(chunk, skip_special_tokens=True)
            translated_chunks.append(translate_one(tokenizer, model, device, chunk_text, generation))
        translated_parts.append(" ".join(translated_chunks))
    return "".join(translated_parts)


def main():
    args = parse_args()
    os.makedirs(args.cache_dir, exist_ok=True)
    tokenizer, model, device = load_model(args.model, args.cache_dir)
    if args.download_only:
        print(json.dumps({"status": "ready", "model": args.model}), flush=True)
        return

    for line in sys.stdin:
        try:
            request = json.loads(line)
            result = translate(
                tokenizer,
                model,
                device,
                args.model,
                request.get("text", ""),
                request.get("source", "en"),
                request.get("target", "zh"),
            )
            response = {"id": request.get("id"), "text": result, "error": ""}
        except Exception as exc:  # Keep the worker alive for recoverable requests.
            response = {"id": request.get("id") if "request" in locals() else None, "text": "", "error": str(exc)}
        print(json.dumps(response, ensure_ascii=False), flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception:
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
