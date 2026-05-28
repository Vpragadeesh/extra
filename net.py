#!/usr/bin/env python3
"""
High-speed data downloader converted from net.c to Python.

Features:
- Parallel downloads from multiple test URLs
- Keyboard controls: 'p' pause/resume, 'q' quit
- Progress bar with ETA, speed, total downloaded

This implementation uses `requests` and `threading`.
"""

from __future__ import annotations

import atexit
import signal
import sys
import threading
import time
from typing import Optional

try:
    from yt_dlp import YoutubeDL
except Exception:
    print("yt-dlp (python package 'yt-dlp') is required: pip install yt-dlp")
    raise
import os

try:
    import termios
    import tty
    import select
except Exception:
    termios = None  # type: ignore

# Target: 600GB
TARGET_GB = 600
TARGET_BYTES = TARGET_GB * 1024 * 1024 * 1024
CHUNK_SIZE = 512 * 1024  # 512KB (kept for compatibility but unused by yt-dlp path)
THREADS_PER_URL = 4

TEST_FILES = [
    "http://speedtest.tele2.net/10GB.zip",
    "http://speedtest.tele2.net/1GB.zip",
    "http://proof.ovh.net/files/10Gb.dat",
    "http://proof.ovh.net/files/1Gb.dat",
    "http://speedtest.belwue.net/100M",
    "http://speedtest-sgp1.digitalocean.com/10gb.test",
    "http://speedtest-ams2.digitalocean.com/10gb.test",
    "http://speedtest.ftp.otenet.gr/files/test10Gb.db",
]

NUM_URLS = len(TEST_FILES)
NUM_THREADS = NUM_URLS * THREADS_PER_URL

# Shared state
total_bytes = 0
total_lock = threading.Lock()
stop_event = threading.Event()
pause_event = threading.Event()
print_lock = threading.Lock()

# Terminal state
_old_termios = None


def format_bytes(num: int) -> str:
    units = ["B", "KB", "MB", "GB", "TB", "PB"]
    value = float(num)
    idx = 0
    while value >= 1024.0 and idx < len(units) - 1:
        value /= 1024.0
        idx += 1
    return f"{value:.2f} {units[idx]}"


def format_speed(bytes_per_sec: float) -> str:
    mbps = bytes_per_sec / (1024.0 * 1024.0)
    return f"{mbps:.1f} MB/s"


def format_eta(seconds: float) -> str:
    if seconds < 0 or seconds > 365 * 24 * 3600:
        return "--:--:--"
    hours = int(seconds / 3600)
    mins = int((seconds - hours * 3600) / 60)
    secs = int(seconds - hours * 3600 - mins * 60)
    return f"{hours:02d}:{mins:02d}:{secs:02d}"


def download_worker(url: str, thread_id: int) -> None:
    """Download a single URL using yt-dlp and report progress via progress hook.

    The hook receives incremental downloaded bytes and updates the global counter.
    """
    global total_bytes

    # per-thread last seen downloaded bytes (from yt-dlp progress dict)
    last_seen = {"bytes": 0}

    def progress_hook(d: dict) -> None:
        # Called frequently by yt-dlp with status updates
        # respect pause: block here while paused
        while pause_event.is_set() and not stop_event.is_set():
            time.sleep(0.1)

        if d.get("status") not in ("downloading",):
            return

        downloaded = d.get("downloaded_bytes") or 0

        # compute delta since last report for this thread
        delta = int(downloaded - last_seen["bytes"])
        if delta > 0:
            # update global total_bytes safely
            global total_bytes
            with total_lock:
                total_bytes += delta
            last_seen["bytes"] = downloaded

            # If target reached, signal stop to others and raise to abort
            if total_bytes >= TARGET_BYTES:
                stop_event.set()
                raise Exception("Target reached - aborting yt-dlp")

        # if caller requested stop, abort by raising
        if stop_event.is_set():
            raise Exception("Stopped by user")

    # Build options for yt-dlp. Write output to os.devnull to avoid disk usage.
    outtmpl = os.devnull
    ydl_opts = {
        "outtmpl": outtmpl,
        "noplaylist": True,
        "quiet": True,
        "no_warnings": True,
        "progress_hooks": [progress_hook],
        # retries help transient network errors
        "retries": 3,
    }

    while not stop_event.is_set():
        try:
            with YoutubeDL(ydl_opts) as ydl:
                # yt-dlp raises on error; this call blocks until completion
                ydl.download([url])
        except Exception:
            # brief pause on error then retry
            time.sleep(1)
            continue


def keyboard_listener() -> None:
    # If termios isn't available, nothing to do
    if termios is None:
        return

    fd = sys.stdin.fileno()
    while not stop_event.is_set():
        dr, _, _ = select.select([sys.stdin], [], [], 0.1)
        if dr:
            c = sys.stdin.read(1)
            if not c:
                continue
            if c.lower() == "p":
                # toggle pause
                if pause_event.is_set():
                    pause_event.clear()
                    with print_lock:
                        print("\n▶️  RESUMED - Press 'p' to pause")
                else:
                    pause_event.set()
                    with print_lock:
                        print("\n⏸️  PAUSED - Press 'p' to resume")
            elif c.lower() == "q":
                with print_lock:
                    print("\n🛑 Stopping downloads...")
                stop_event.set()
                break


def restore_terminal() -> None:
    global _old_termios
    if _old_termios is not None and termios is not None:
        termios.tcsetattr(sys.stdin.fileno(), termios.TCSADRAIN, _old_termios)


def setup_terminal() -> bool:
    global _old_termios
    if termios is None:
        return False
    try:
        _old_termios = termios.tcgetattr(sys.stdin.fileno())
        new = _old_termios[:]
        # turn off canonical mode and echo
        new[3] = new[3] & ~(termios.ICANON | termios.ECHO)
        # VMIN = 0, VTIME = 0
        new[6][termios.VMIN] = 0
        new[6][termios.VTIME] = 0
        termios.tcsetattr(sys.stdin.fileno(), termios.TCSANOW, new)
        atexit.register(restore_terminal)
        return True
    except Exception:
        return False


def print_progress(
    bytes_downloaded: int,
    total: int,
    speed: float,
    elapsed_hours: float,
    eta_seconds: float,
    paused: bool,
) -> None:
    bar_width = 30
    progress = float(bytes_downloaded) / float(total) if total > 0 else 0.0
    filled = int(progress * bar_width)

    bytes_str = format_bytes(bytes_downloaded)
    total_str = format_bytes(total)
    speed_str = format_speed(speed)
    eta_str = format_eta(eta_seconds)

    with print_lock:
        sys.stdout.write("\r")
        if paused:
            sys.stdout.write("⏸️  PAUSED : ")
        else:
            sys.stdout.write("📥 Progress: ")

        sys.stdout.write(f"{progress * 100:5.1f}%|")
        sys.stdout.write("".join("█" if i < filled else "░" for i in range(bar_width)))
        sys.stdout.write(
            f"| {bytes_str}/{total_str} | 🚀 {speed_str} | ⏱️ {elapsed_hours:.2f}h | ETA: {eta_str}"
        )
        if paused:
            sys.stdout.write(" (paused)")
        sys.stdout.write("    ")
        sys.stdout.flush()


def signal_handler(sig_num, frame) -> None:  # pragma: no cover - interactive
    stop_event.set()


def main() -> None:
    global total_bytes

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    term_ok = setup_terminal()
    if not term_ok:
        print("ℹ️  Keyboard controls not available in this terminal")

    print("============================================================")
    print("🚀 HIGH-SPEED DATA DOWNLOADER (Python Version)")
    print(f"🎯 Target: {TARGET_GB} GB")
    print(f"📡 Using {NUM_URLS} download sources with {THREADS_PER_URL} threads each")
    print("============================================================")
    print("⚠️  Data is downloaded to RAM and discarded (no disk usage)")
    print("============================================================")
    print("🎮 Controls: Press 'p' to PAUSE/PLAY, 'q' to QUIT")
    print("============================================================\n")

    start_time = time.time()

    # Start keyboard listener thread
    kb_thread = threading.Thread(target=keyboard_listener, daemon=True)
    kb_thread.start()

    # Start download threads
    threads: list[threading.Thread] = []
    thread_id = 0
    for url in TEST_FILES:
        for _ in range(THREADS_PER_URL):
            t = threading.Thread(
                target=download_worker, args=(url, thread_id), daemon=True
            )
            t.start()
            threads.append(t)
            thread_id += 1

    print(f"Started {len(threads)} download threads\n")

    last_bytes = 0
    avg_speed_smooth = 0.0
    pause_time = 0.0
    pause_start: Optional[float] = None

    while not stop_event.is_set():
        current_bytes = 0
        with total_lock:
            current_bytes = total_bytes

        # Check if target reached
        if current_bytes >= TARGET_BYTES:
            stop_event.set()
            break

        # Pause tracking
        if pause_event.is_set():
            if pause_start is None:
                pause_start = time.time()
        else:
            if pause_start is not None:
                pause_time += time.time() - pause_start
                pause_start = None

        elapsed = time.time() - start_time - pause_time
        elapsed_hours = elapsed / 3600.0 if elapsed > 0 else 0.0

        # speed = bytes in last second
        speed = float(current_bytes - last_bytes)

        if avg_speed_smooth == 0.0 and speed > 0:
            avg_speed_smooth = speed
        elif speed > 0:
            avg_speed_smooth = avg_speed_smooth * 0.9 + speed * 0.1

        eta_seconds = -1.0
        if avg_speed_smooth > 0:
            remaining = max(0, TARGET_BYTES - current_bytes)
            eta_seconds = remaining / avg_speed_smooth

        print_progress(
            current_bytes,
            TARGET_BYTES,
            speed,
            elapsed_hours,
            eta_seconds,
            pause_event.is_set(),
        )

        last_bytes = current_bytes
        time.sleep(1.0)

    # Wait for threads to finish
    print("\n\nWaiting for threads to finish...")
    for t in threads:
        t.join(timeout=0.1)

    kb_thread.join(timeout=0.1)

    final_bytes = 0
    with total_lock:
        final_bytes = total_bytes

    elapsed = time.time() - start_time - pause_time
    hours = elapsed / 3600.0 if elapsed > 0 else 0.0
    avg_speed = float(final_bytes) / elapsed if elapsed > 0 else 0.0

    print("\n============================================================")
    if final_bytes >= TARGET_BYTES:
        print(f"🎉 TARGET REACHED: {TARGET_GB} GB Downloaded!")
    else:
        print(f"📊 Downloaded: {format_bytes(final_bytes)}")
    print("============================================================")
    print(f"📥 Total Downloaded: {format_bytes(final_bytes)}")
    print(f"⏱️  Time Taken: {hours:.2f} hours (excluding pauses)")
    print(f"⏸️  Total Pause Time: {pause_time / 60.0:.1f} minutes")
    print(f"📈 Average Speed: {format_speed(avg_speed)}")
    print("============================================================")


if __name__ == "__main__":
    main()
