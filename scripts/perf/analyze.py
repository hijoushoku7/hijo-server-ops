#!/usr/bin/env python3
"""bench.py の CSV を集計する。

読む場所は既定でこのスクリプトと同じディレクトリ。HSO_PERF_OUT で上書きできる。
"""
import csv
import json
import os
import statistics
import sys

OUT = os.environ.get("HSO_PERF_OUT") or os.path.dirname(os.path.abspath(__file__))
HZ = 100


def load(label):
    rows = []
    with open(os.path.join(OUT, f"{label}.csv")) as f:
        for row in csv.DictReader(f):
            row["t"] = float(row["t"])
            for key in ("rss_kb", "pss_kb", "hwm_kb", "vsz_kb", "threads",
                        "cpu_jiffies", "mem_available_kb", "pid"):
                row[key] = int(row[key])
            rows.append(row)
    return rows


def summarize(label):
    rows = load(label)
    meta = json.load(open(os.path.join(OUT, f"{label}.meta.json")))
    measure = [r for r in rows if r["phase"] == "measure"]
    startup = [r for r in rows if r["phase"] == "startup"]
    result = {"label": label, "ready_sec": meta["ready_sec"], "roles": {}}

    span = measure[-1]["t"] - measure[0]["t"]
    for role in sorted({r["role"] for r in measure}):
        sel = [r for r in measure if r["role"] == role]
        by_pid = {}
        for r in sel:
            by_pid.setdefault(r["pid"], []).append(r)
        cpu = 0.0
        for pid, series in by_pid.items():
            cpu += series[-1]["cpu_jiffies"] - series[0]["cpu_jiffies"]
        rss = [r["rss_kb"] for r in sel]
        pss = [r["pss_kb"] for r in sel if r["pss_kb"] >= 0]
        # 起動フェーズ含む全体のピーク
        all_role = [r for r in rows if r["role"] == role]
        result["roles"][role] = {
            "rss_mean_mb": statistics.mean(rss) / 1024,
            "rss_max_mb": max(rss) / 1024,
            "rss_min_mb": min(rss) / 1024,
            "rss_first_mb": rss[0] / 1024,
            "rss_last_mb": rss[-1] / 1024,
            "pss_mean_mb": (statistics.mean(pss) / 1024) if pss else None,
            "hwm_max_mb": max(r["hwm_kb"] for r in all_role) / 1024,
            "vsz_mean_mb": statistics.mean(r["vsz_kb"] for r in sel) / 1024,
            "threads_mean": statistics.mean(r["threads"] for r in sel),
            "cpu_pct": cpu / HZ / span * 100,
            "cpu_sec_total": cpu / HZ,
            "samples": len(sel),
        }

    # ツリー合計（同一タイムスタンプごとに合算）
    per_t = {}
    for r in measure:
        per_t.setdefault(r["t"], [0, 0])
        per_t[r["t"]][0] += r["rss_kb"]
        if r["pss_kb"] >= 0:
            per_t[r["t"]][1] += r["pss_kb"]
    result["tree_rss_mean_mb"] = statistics.mean(v[0] for v in per_t.values()) / 1024
    result["tree_rss_max_mb"] = max(v[0] for v in per_t.values()) / 1024
    result["tree_pss_mean_mb"] = statistics.mean(v[1] for v in per_t.values()) / 1024
    result["measure_span_sec"] = span

    # 起動フェーズの CPU（プロセス全体、開始から ready まで）
    if startup:
        first = {}
        last = {}
        for r in rows:
            if r["phase"] in ("startup",):
                first.setdefault(r["pid"], r["cpu_jiffies"])
                last[r["pid"]] = r["cpu_jiffies"]
        result["startup_cpu_sec"] = sum(last[p] - first[p] for p in last) / HZ

    # java 単体の RSS 推移（測定区間の傾き）
    java = [r for r in measure if r["role"] == "java"]
    if java:
        result["java_rss_drift_mb"] = (java[-1]["rss_kb"] - java[0]["rss_kb"]) / 1024
    hso = [r for r in measure if r["role"] == "hso-ui"]
    if hso:
        result["hso_rss_drift_mb"] = (hso[-1]["rss_kb"] - hso[0]["rss_kb"]) / 1024
    return result


if __name__ == "__main__":
    for label in sys.argv[1:]:
        print(json.dumps(summarize(label), indent=2, ensure_ascii=False))
