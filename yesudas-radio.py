#!/usr/bin/env python3
import argparse
import datetime
import os
import shutil
import subprocess
import sys
from pathlib import Path

# ----------------------------------------------------------------------
#  Default values – tweak these if you prefer hard‑coded ones
# ----------------------------------------------------------------------
DEFAULT_STREAM_URL = "https://stream-169.zeno.fm/uvdbygm6a48uv?zt=..."
DEFAULT_LOG_FILE = Path.home() / "yesudas-radio-log.txt"
RADIO_NAME = "KJ Yesudas Radio"

# ----------------------------------------------------------------------
#  Helper – detect whether mpv is available
# ----------------------------------------------------------------------
def check_mpv():
    if shutil.which("mpv") is None:
        sys.exit("❌  mpv was not found in your PATH. Install it first.\n")
    return True

# ----------------------------------------------------------------------
#  Parse command line arguments
# ----------------------------------------------------------------------
def parse_args():
    parser = argparse.ArgumentParser(
        description="Play a radio stream with mpv and log the song titles.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    parser.add_argument(
        "--url",
        "-u",
        default=DEFAULT_STREAM_URL,
        help="The streaming URL (e.g. the Zeno FM link).",
    )
    parser.add_argument(
        "--log",
        "-l",
        type=Path,
        default=DEFAULT_LOG_FILE,
        help="Path to the log file that will receive the timestamps and titles.",
    )
    return parser.parse_args()

# ----------------------------------------------------------------------
#  Main routine
# ----------------------------------------------------------------------
def main():
    args = parse_args()
    check_mpv()

    stream_url = args.url
    log_file = args.log

    print(f"🎵 Now playing: {RADIO_NAME}")
    print(f"📝 Logging songs to: {log_file}\n")

    # Make sure the log directory exists
    log_file.parent.mkdir(parents=True, exist_ok=True)

    # Start mpv as a subprocess, capturing stdout + stderr
    try:
        proc = subprocess.Popen(
            ["mpv", stream_url],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,          # decode bytes to str automatically
            bufsize=1,          # line buffered
        )
    except Exception as exc:
        sys.exit(f"❌  Failed to start mpv: {exc}")

    # Open the log file once and keep it open – it will automatically
    # append (`a`) new lines and flush after each write
    with log_file.open("a", encoding="utf-8") as lf:
        try:
            for line in proc.stdout:
                # mpv emits many lines; we only care about the title
                if "icy-title:" in line:
                    # Extract the part after the colon
                    title = line.split("icy-title:", 1)[1].strip()
                    timestamp = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
                    log_entry = f"[{timestamp}] {title}\n"
                    lf.write(log_entry)
                    lf.flush()  # make sure it lands on disk

                    # Optional: nicer console notification
                    print(f"🎶 {title}")
        except KeyboardInterrupt:
            print("\n⏹  Stopping…")
            proc.terminate()
            proc.wait()
            sys.exit(0)

    # mpv finished – wait for the process to exit cleanly
    proc.wait()
    print("\n🛑 mpv finished, script exiting.")

if __name__ == "__main__":
    main()
