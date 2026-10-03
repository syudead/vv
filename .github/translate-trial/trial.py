"""Try translation-specialised models on English sample segments.

Runs on a GitHub-hosted runner: finds a GGUF file for the requested model on
Hugging Face, serves it with llama-server, translates the samples into
Japanese with the model's own prompt format, and writes a Markdown report
with the output, the tag check and the generation speed.
"""

import json
import os
import re
import subprocess
import sys
import time
import urllib.parse
import urllib.request

HF = "https://huggingface.co"

SAMPLES = [
    ("heading", "plain", "Where translations live, and how they follow the English source", None),
    ("paragraph", "plain",
     "Translations are generated on main and stored on a bot-owned orphan branch. "
     "They are published as the site's Japanese locale and are never edited by hand.", None),
    ("link and code", "format",
     "Read <s1>ARCHITECTURE.md</s1> for system boundaries and dependency direction "
     "before you change the <s2>internal/httpapi</s2> package.", None),
    ("technical", "plain",
     "While the pointer hovers over the seek bar, the player shows a frame from the "
     "sprite sheet. The frame is chosen from the sprite interval and clamped to the last frame.", None),
    ("terminology", "term",
     "Open the Versions menu, then confirm or reject each tentative tag.",
     [("Versions", "Versions"), ("tentative tag", "仮のタグ")]),
    ("table cell", "plain", "Fails red and lists the files in its summary", None),
    ("list item", "plain",
     "A file that is still listed as Japanese source is skipped, because it has no "
     "English source yet.", None),
    ("long paragraph", "plain",
     "The English site deploys at once and never waits for the translation, which can "
     "run for hours on the first pass. When the translation ends, a second build deploys "
     "whatever the translation branch then holds. Both deploys share one concurrency "
     "group, so they run in order, and a failed translation never blocks publishing.", None),
]


def get_json(url):
    req = urllib.request.Request(url, headers={"User-Agent": "vv-translate-trial"})
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)


def find_gguf(query, quants, author_first):
    url = f"{HF}/api/models?" + urllib.parse.urlencode(
        {"search": query, "filter": "gguf", "sort": "downloads", "direction": "-1", "limit": "30"})
    repos = [m["id"] for m in get_json(url)]
    repos.sort(key=lambda r: 0 if r.split("/")[0].lower() == author_first else 1)
    print(f"candidate repos for {query!r}: {repos}", flush=True)
    for quant in quants:
        for repo in repos:
            files = [s["rfilename"] for s in get_json(f"{HF}/api/models/{repo}").get("siblings", [])]
            for f in files:
                name = f.lower()
                if name.endswith(".gguf") and quant.lower() in name and "mmproj" not in name \
                        and not re.search(r"-\d{5}-of-\d{5}", name):
                    return repo, f
    raise SystemExit(f"no GGUF file found for {query!r} with {quants}")


def post(path, body):
    req = urllib.request.Request(
        "http://127.0.0.1:8080" + path, data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=1800) as r:
        return json.load(r)


def hy_prompt(kind, text, terms):
    if kind == "term":
        ref = "\n".join(f"{s} 翻译成 {t}" for s, t in terms)
        return (f"参考下面的翻译：\n{ref}\n\n"
                f"将以下文本翻译为日语，注意只需要输出翻译后的结果，不要额外解释：\n{text}")
    if kind == "format":
        return ("将以下<source></source>之间的文本翻译为日语，注意只需要输出翻译后的结果，不要额外解释，"
                "原文中的<sn></sn>标签表示标签内文本包含格式信息，需要在译文中相应的位置尽量保留该标签。"
                "输出格式为：<target>str</target>\n\n"
                f"<source>{text}</source>")
    return f"Translate the following segment into Japanese, without additional explanation.\n\n{text}"


def generic_prompt(kind, text, terms):
    rules = ""
    if kind == "term":
        rules = "Use these term translations: " + "; ".join(f"{s} -> {t}" for s, t in terms) + "\n"
    if kind == "format":
        rules = "Keep every <sN>...</sN> tag around the corresponding translated words.\n"
    return f"Translate the following text from English to Japanese.\n{rules}\n{text}"


def translate(family, kind, text, terms):
    if family == "plamo":
        src = text
        if kind == "term":
            src = text  # plamo has no term template; terms are checked afterwards
        prompt = ("<|plamo:op|>dataset\ntranslation\n"
                  f"<|plamo:op|>input lang=English\n{src}\n"
                  "<|plamo:op|>output lang=Japanese\n")
        r = post("/completion", {"prompt": prompt, "n_predict": 512, "temperature": 0,
                                 "stop": ["<|plamo:op|>"]})
        return r["content"].strip(), r.get("timings", {})
    if family == "gemma":
        # TranslateGemma's chat template only accepts typed content, so the
        # official prompt is sent as a raw Gemma turn.
        note = ""
        if kind == "term":
            note = " Use these term translations: " + "; ".join(f"{s} -> {t}" for s, t in terms) + "."
        if kind == "format":
            note = " Keep every <sN>...</sN> tag around the corresponding translated words."
        prompt = ("<start_of_turn>user\nYou are a professional English (en) to Japanese (ja) translator. "
                  "Your goal is to accurately convey the meaning and nuances of the original English text "
                  "while adhering to Japanese grammar, vocabulary, and cultural sensitivities. Produce only "
                  "the Japanese translation, without any additional explanations or commentary." + note +
                  " Please translate the following English text into Japanese:\n\n\n"
                  f"{text}<end_of_turn>\n<start_of_turn>model\n")
        r = post("/completion", {"prompt": prompt, "n_predict": 512, "temperature": 0,
                                 "stop": ["<end_of_turn>"]})
        return r["content"].strip(), r.get("timings", {})
    prompt = hy_prompt(kind, text, terms) if family == "hy" else generic_prompt(kind, text, terms)
    params = ({"temperature": 0.7, "top_p": 0.6, "top_k": 20, "repeat_penalty": 1.05}
              if family == "hy" else {"temperature": 0})
    r = post("/v1/chat/completions", {"messages": [{"role": "user", "content": prompt}],
                                      "max_tokens": 512, **params})
    return r["choices"][0]["message"]["content"].strip(), r.get("timings", {})


def tags_ok(src, out):
    want = re.findall(r"</?s\d+>", src)
    return sorted(want) == sorted(re.findall(r"</?s\d+>", out))


def main():
    name, family, query, author = sys.argv[1:5]
    quants = sys.argv[5].split(",")
    report = [f"## {name}", ""]
    try:
        repo, fname = find_gguf(query, quants, author)
    except Exception as e:  # report and stop; another model may still work
        report += [f"Model not found: {e}", ""]
        write(report)
        return
    report += [f"Model file: `{repo}/{fname}`", ""]
    print(f"downloading {repo}/{fname}", flush=True)
    t0 = time.time()
    subprocess.run(["curl", "-sSfL", "-o", "model.gguf",
                    f"{HF}/{repo}/resolve/main/{urllib.parse.quote(fname)}"], check=True)
    size = os.path.getsize("model.gguf") / 1e9
    report += [f"Size: {size:.2f} GB, download {time.time() - t0:.0f} s", ""]

    jinja = "--no-jinja" if family == "gemma" else "--jinja"
    server = subprocess.Popen(["./llama/llama-server", "-m", "model.gguf", "-c", "4096",
                               "-t", str(os.cpu_count()), "--port", "8080", jinja],
                              stdout=open("server.log", "w"), stderr=subprocess.STDOUT)
    for _ in range(600):
        try:
            urllib.request.urlopen("http://127.0.0.1:8080/health", timeout=2)
            break
        except Exception:
            if server.poll() is not None:
                break
            time.sleep(1)
    if server.poll() is not None:
        report += ["llama-server did not start:", "", "```text",
                   open("server.log").read()[-3000:], "```", ""]
        write(report)
        return

    report += ["| Sample | Source | Output | Tags kept | Terms kept | Tokens/s |",
               "| --- | --- | --- | --- | --- | --- |"]
    speeds = []
    for label, kind, text, terms in SAMPLES:
        try:
            out, timing = translate(family, kind, text, terms)
        except Exception as e:
            out, timing = f"ERROR: {e}", {}
        tps = timing.get("predicted_per_second")
        if tps:
            speeds.append(tps)
        tag = ("yes" if tags_ok(text, out) else "**no**") if kind == "format" else ""
        term = ""
        if terms:
            term = "yes" if all(t in out for _, t in terms) else "**no**"
        cell = lambda s: s.replace("|", "\\|").replace("\n", "<br>")
        report.append(f"| {label} | {cell(text)} | {cell(out)} | {tag} | {term} | "
                      f"{tps:.1f} |" if tps else
                      f"| {label} | {cell(text)} | {cell(out)} | {tag} | {term} | |")
        print(f"[{label}] {out}", flush=True)
    if speeds:
        report += ["", f"Mean generation speed: {sum(speeds) / len(speeds):.1f} tokens/s "
                       f"on {os.cpu_count()} CPUs", ""]
    server.terminate()
    write(report)


def write(lines):
    text = "\n".join(lines) + "\n"
    print(text)
    with open("report.md", "w") as f:
        f.write(text)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as f:
            f.write(text)


if __name__ == "__main__":
    main()
