#!/usr/bin/env python3
"""Start a real executable with isolated storage; optional user-mode emulator.
This checks HTTP, SQLite configuration persistence and SIGTERM, not market coverage.
"""
import argparse
import json
import pathlib
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("binary", type=pathlib.Path)
parser.add_argument("--emulator", type=pathlib.Path)
parser.add_argument("--report", type=pathlib.Path)
args = parser.parse_args()
command = ([str(args.emulator.resolve())] if args.emulator else []) + [str(args.binary.resolve())]
report = {"binary": str(args.binary), "emulator": str(args.emulator) if args.emulator else None, "runs": []}
with tempfile.TemporaryDirectory(prefix="market-mood-binary-smoke-") as directory:
    for run in range(2):
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", 0))
            port = probe.getsockname()[1]
        base = f"http://127.0.0.1:{port}"
        with open(pathlib.Path(directory) / f"run-{run}.log", "w+") as log:
            process = subprocess.Popen(command + ["--bind", f"127.0.0.1:{port}", "--data-dir", directory], stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 30
                while True:
                    if process.poll() is not None:
                        raise RuntimeError(f"server exited {process.returncode}")
                    try:
                        with urllib.request.urlopen(base + "/", timeout=2) as response:
                            html = response.read()
                            assert response.status == 200 and b"<html" in html
                        break
                    except OSError:
                        if time.monotonic() >= deadline:
                            raise
                        time.sleep(0.1)
                def get(path):
                    with urllib.request.urlopen(base + path, timeout=10) as response:
                        assert response.status == 200
                        return json.load(response)
                tasks = get("/api/tasks")["tasks"]
                assert len(tasks) == 10
                assert all(task["state"] == "running" for task in tasks)
                database = get("/api/database")
                assert database["pool"]["max_size"] == 4
                dashboard = get("/api/dashboard")
                assert len(dashboard["pools"]) == 7
                saved = {"quotes": ["eastmoney"], "stock_quotes": ["tencent"]}
                if run == 0:
                    request = urllib.request.Request(base + "/api/sources", data=json.dumps(saved).encode(), headers={"Content-Type": "application/json"}, method="PUT")
                    with urllib.request.urlopen(request, timeout=10) as response:
                        assert json.load(response)["ok"]
                sources = get("/api/sources")
                assert all(sources[key] == value for key, value in saved.items())
                started = time.monotonic()
                process.send_signal(signal.SIGTERM)
                code = process.wait(timeout=13)
                assert code == 0, f"exit {code}"
                report["runs"].append({"homepage_bytes": len(html), "task_count": len(tasks), "config_persisted": True, "exit_code": code, "shutdown_seconds": round(time.monotonic() - started, 3)})
            except BaseException:
                log.flush()
                log.seek(0)
                print(log.read()[-8000:])
                raise
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait()
report["passed"] = True
output = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
if args.report:
    args.report.write_text(output)
print(output)
