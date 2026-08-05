#!/usr/bin/env python3
"""hso のオーバーヘッド計測。

usage: bench.py {bare|hso} <label> <settle_sec> <sample_sec>

bare : mc-server-test/run.sh を直接起動（stdout はファイル）
hso  : pty 上で hso_ja -config hso.toml を起動

出力: <label>.csv (毎秒のプロセス別サンプル), <label>.meta.json
"""
import json
import os
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import termios
import time
import fcntl

ROOT = "/home/hijoushoku9/projects/hijo-server-ops"
SERVER_DIR = os.path.join(ROOT, "mc-server-test")
LOG = os.path.join(SERVER_DIR, "logs", "latest.log")
OUT = "/home/hijoushoku9/.claude/jobs/bde3b859/tmp"
HZ = os.sysconf("SC_CLK_TCK")
PAGE = os.sysconf("SC_PAGE_SIZE")

stat_re = re.compile(r"^(\d+) \((.*)\) (\S) (.*)$", re.S)


def read_stat(pid):
    try:
        with open(f"/proc/{pid}/stat") as f:
            data = f.read()
    except OSError:
        return None
    m = stat_re.match(data)
    if not m:
        return None
    comm = m.group(2)
    rest = m.group(4).split()
    # rest[0] は ppid（stat の 4 番目のフィールド）
    return {
        "pid": int(m.group(1)),
        "comm": comm,
        "ppid": int(rest[0]),
        "utime": int(rest[10]),
        "stime": int(rest[11]),
        "num_threads": int(rest[16]),
        "rss_pages": int(rest[20]),
    }


def read_status(pid):
    out = {}
    try:
        with open(f"/proc/{pid}/status") as f:
            for line in f:
                if line.startswith(("VmRSS:", "VmHWM:", "VmSize:", "RssAnon:", "RssFile:", "RssShmem:")):
                    key, value = line.split(":", 1)
                    out[key] = int(value.split()[0])  # kB
    except OSError:
        pass
    return out


def read_pss(pid):
    try:
        with open(f"/proc/{pid}/smaps_rollup") as f:
            for line in f:
                if line.startswith("Pss:"):
                    return int(line.split()[1])
    except OSError:
        pass
    return None


def cmdline(pid):
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as f:
            return f.read().replace(b"\0", b" ").decode(errors="replace").strip()
    except OSError:
        return ""


def descendants(root):
    procs = {}
    for name in os.listdir("/proc"):
        if not name.isdigit():
            continue
        st = read_stat(int(name))
        if st:
            procs[st["pid"]] = st
    children = {}
    for st in procs.values():
        children.setdefault(st["ppid"], []).append(st["pid"])
    found, stack = [], [root]
    while stack:
        pid = stack.pop()
        if pid in procs:
            found.append(procs[pid])
            stack.extend(children.get(pid, []))
    return found


def classify(st, cmd):
    if "__hso_supervise" in cmd:
        return "hso-supervisor"
    if st["comm"] == "java" or "/java" in cmd:
        return "java"
    if "hso_ja" in cmd or "hso_en" in cmd:
        return "hso-ui"
    return st["comm"]


def meminfo():
    out = {}
    with open("/proc/meminfo") as f:
        for line in f:
            key, value = line.split(":", 1)
            if key in ("MemAvailable", "MemFree", "Cached"):
                out[key] = int(value.split()[0])
    return out


def log_inode():
    try:
        return os.stat(LOG).st_ino
    except OSError:
        return None


def wait_ready(old_inode, timeout=300):
    deadline = time.time() + timeout
    while time.time() < deadline:
        ino = log_inode()
        if ino is not None and ino != old_inode:
            try:
                with open(LOG, errors="replace") as f:
                    if "Done (" in f.read():
                        return True
            except OSError:
                pass
        time.sleep(0.5)
    return False


def start_bare():
    stdout = open(os.path.join(OUT, "bare-server.out"), "wb")
    proc = subprocess.Popen(
        ["./run.sh"],
        cwd=SERVER_DIR,
        stdin=subprocess.PIPE,
        stdout=stdout,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    return proc, None


def start_hso():
    pid, fd = pty.fork()
    if pid == 0:
        os.chdir(SERVER_DIR)
        os.environ["TERM"] = "xterm-256color"
        os.environ["COLUMNS"] = "120"
        os.environ["LINES"] = "40"
        os.execv(os.path.join(ROOT, "hso_ja"),
                 [os.path.join(ROOT, "hso_ja"), "-config", "hso.toml"])
        os._exit(127)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    return pid, fd


def drain(fd, sink):
    """pty の出力を読み捨てる。詰まると hso 側が止まるので必ず読む。"""
    while True:
        r, _, _ = select.select([fd], [], [], 0)
        if not r:
            return
        try:
            data = os.read(fd, 65536)
        except OSError:
            return
        if not data:
            return
        sink.write(data)


def main():
    mode, label, settle, sample = sys.argv[1], sys.argv[2], float(sys.argv[3]), float(sys.argv[4])
    old_inode = log_inode()
    t_start = time.time()

    sink = open(os.path.join(OUT, f"{label}.raw"), "wb")
    if mode == "bare":
        proc, fd = start_bare()
        root = proc.pid
    else:
        root, fd = start_hso()
        proc = None

    csv = open(os.path.join(OUT, f"{label}.csv"), "w")
    csv.write("t,phase,role,pid,rss_kb,pss_kb,hwm_kb,vsz_kb,threads,cpu_jiffies,mem_available_kb\n")

    prev = {}

    def tick(phase):
        now = time.time() - t_start
        mi = meminfo()
        for st in descendants(root):
            cmd = cmdline(st["pid"])
            role = classify(st, cmd)
            status = read_status(st["pid"])
            pss = read_pss(st["pid"])
            csv.write("%.1f,%s,%s,%d,%d,%s,%d,%d,%d,%d,%d\n" % (
                now, phase, role, st["pid"],
                status.get("VmRSS", st["rss_pages"] * PAGE // 1024),
                pss if pss is not None else -1,
                status.get("VmHWM", -1),
                status.get("VmSize", -1),
                st["num_threads"],
                st["utime"] + st["stime"],
                mi.get("MemAvailable", -1),
            ))
        csv.flush()

    # 起動フェーズ
    ready = False
    deadline = time.time() + 300
    while time.time() < deadline:
        if fd is not None:
            drain(fd, sink)
        tick("startup")
        ino = log_inode()
        if ino is not None and ino != old_inode:
            try:
                with open(LOG, errors="replace") as f:
                    if "Done (" in f.read():
                        ready = True
                        break
            except OSError:
                pass
        time.sleep(1.0)
    ready_at = time.time() - t_start

    # 安定待ち
    end = time.time() + settle
    while time.time() < end:
        if fd is not None:
            drain(fd, sink)
        tick("settle")
        time.sleep(1.0)

    # 計測
    end = time.time() + sample
    while time.time() < end:
        if fd is not None:
            drain(fd, sink)
        tick("measure")
        time.sleep(1.0)

    # 停止
    stop_at = time.time() - t_start
    if mode == "bare":
        proc.stdin.write(b"stop\n")
        proc.stdin.flush()
        try:
            proc.wait(timeout=120)
        except subprocess.TimeoutExpired:
            proc.kill()
    else:
        os.write(fd, b"\x03")
        deadline = time.time() + 120
        while time.time() < deadline:
            drain(fd, sink)
            pid_done, _ = os.waitpid(root, os.WNOHANG)
            if pid_done:
                break
            time.sleep(0.5)
        else:
            os.kill(root, signal.SIGKILL)

    csv.close()
    sink.close()
    meta = {
        "mode": mode,
        "label": label,
        "ready": ready,
        "ready_sec": ready_at,
        "stop_sec": stop_at,
        "settle": settle,
        "sample": sample,
        "hz": HZ,
        "started_at": t_start,
    }
    with open(os.path.join(OUT, f"{label}.meta.json"), "w") as f:
        json.dump(meta, f, indent=2)
    print(json.dumps(meta))


if __name__ == "__main__":
    main()
