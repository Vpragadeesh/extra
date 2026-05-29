#!/usr/bin/env python3
import os
import sys
import json
import subprocess
from pathlib import Path

OUTPUT_DIR = Path.home() / "Videos" / "youtube-members"
AUDIO_DIR = OUTPUT_DIR / "audio"

# Create directories
OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
AUDIO_DIR.mkdir(parents=True, exist_ok=True)

def print_usage():
    print("Usage:")
    print(f"  {os.path.basename(__file__)} VIDEO_URL")
    print(f"  {os.path.basename(__file__)} -a urls.txt")


def run_yt_dlp(args):
    cmd = ["yt-dlp"] + args
    subprocess.run(cmd)


def download_with_format_selection(url):
    # Detect playlist
    pl_count = 0
    try:
        result = subprocess.run(
            ["yt-dlp", "-J", "--flat-playlist", url],
            capture_output=True, text=True, check=True
        )
        data = json.loads(result.stdout)
        entries = data.get("entries", [])
        if entries:
            pl_count = len(entries)
    except Exception:
        pass

    if pl_count > 0:
        ans = input(f"Detected playlist with {pl_count} items. Download entire playlist? (Y/n) ").strip()
        if not ans:
            ans = "Y"
        if not ans.lower().startswith("y"):
            print("Aborting. If you want to download a single video from the playlist, pass its direct URL instead.")
            return

    # Ask audio or video
    print("\nWhat would you like to download?")
    options = [
        "Video + Audio (1440p if available)",
        "Video + Audio (1080p if available)",
        "Video + Audio (720p if available)",
        "Video + Audio (480p if available)",
        "Video + Audio (360p if available)",
        "Video + Audio (240p if available)",
        "Video + Audio (144p if available)",
        "Audio only (webm/best)",
        "Video + Audio (AV1 best quality, .webm)",
        "Custom format string"
    ]
    for i, opt in enumerate(options, 1):
        print(f"  {i}) {opt}")
    
    choice = input(f"Select (1-{len(options)}, default 1): ").strip()
    if not choice:
        choice = "1"
    try:
        choice = int(choice)
    except ValueError:
        choice = 1

    output_template = str(OUTPUT_DIR / "%(channel)s" / "%(playlist_title|Single Videos)s" / "%(title)s [%(id)s].%(ext)s")
    archive_file = str(OUTPUT_DIR / "downloaded.txt")
    embed_thumb = True
    extract_audio = False
    audio_format = "mp3"
    audio_quality = ""
    merge_fmt = ""

    if choice == 2:
        fmt = "400+251/bestvideo+bestaudio/best"
        desc = "1080p video + audio"
    elif choice == 3:
        fmt = "bestvideo[height<=720]+bestaudio/best[height<=720]"
        desc = "720p video + audio"
    elif choice == 4:
        fmt = "bestvideo[height<=480]+bestaudio/best[height<=480]"
        desc = "480p video + audio"
    elif choice == 5:
        fmt = "bestvideo[height<=360]+bestaudio/best[height<=360]"
        desc = "360p video + audio"
    elif choice == 6:
        fmt = "bestvideo[height<=240]+bestaudio/best[height<=240]"
        desc = "240p video + audio"
    elif choice == 7:
        fmt = "bestvideo[height<=144]+bestaudio/best[height<=144]"
        desc = "144p video + audio"
    elif choice == 8:
        fmt = "bestaudio"
        desc = "audio only (webm/best)"
        output_template = str(AUDIO_DIR / "%(channel)s" / "%(playlist_title|Single Videos)s" / "%(title)s [%(id)s].%(ext)s")
        archive_file = str(AUDIO_DIR / "downloaded.txt")
        embed_thumb = False
    elif choice == 9:
        fmt = "bestvideo[vcodec^=av01]+bestaudio[ext=webm]/bestvideo[ext=webm]+bestaudio[ext=webm]/best"
        merge_fmt = "webm"
        desc = "AV1 video + audio (.webm)"
    elif choice == 10:
        fmt = input("Enter format string (e.g. 308+251 or bestaudio): ").strip()
        merge_fmt = input("Enter merge format (mkv/mp4/webm, default empty): ").strip()
        desc = f"custom ({fmt})"
    else:
        fmt = "308+251/400+251/bestvideo+bestaudio/best"
        desc = "1440p video + audio"

    print(f"\nDownloading: {desc}")
    print(f"URL: {url}\n")

    dl_args = [
        "--cookies-from-browser", "firefox",
        "--remote-components", "ejs:github",
        "-f", fmt,
        "--embed-metadata",
        "--write-subs",
        "--write-auto-subs",
        "--sub-langs", "en.*",
        "--convert-subs", "srt",
        "--download-archive", archive_file,
        "-o", output_template
    ]

    if embed_thumb:
        dl_args.append("--embed-thumbnail")
    if extract_audio:
        dl_args.extend(["--extract-audio", "--audio-format", audio_format, "--audio-quality", audio_quality])
    if merge_fmt:
        dl_args.extend(["--merge-output-format", merge_fmt])

    dl_args.append(url)
    run_yt_dlp(dl_args)


def main():
    if len(sys.argv) == 1:
        print_usage()
        sys.exit(1)

    if sys.argv[1] == "-a":
        if len(sys.argv) < 3:
            print("Missing filename after -a")
            sys.exit(1)
        
        fmt = "308+251/400+251/bestvideo+bestaudio/best"
        archive_file = str(OUTPUT_DIR / "downloaded.txt")
        output_template = str(OUTPUT_DIR / "%(channel)s" / "%(playlist_title|Single Videos)s" / "%(title)s [%(id)s].%(ext)s")
        
        dl_args = [
            "--cookies-from-browser", "firefox",
            "--remote-components", "ejs:github",
            "-f", fmt,
            "--embed-metadata",
            "--embed-thumbnail",
            "--write-subs",
            "--write-auto-subs",
            "--sub-langs", "en.*",
            "--convert-subs", "srt",
            "--download-archive", archive_file,
            "-o", output_template,
            "-a", sys.argv[2]
        ]
        run_yt_dlp(dl_args)
        
    elif sys.stdin.isatty() and sys.stdout.isatty():
        download_with_format_selection(sys.argv[1])
        
    else:
        # Non-interactive single URL
        fmt = "308+251/400+251/bestvideo+bestaudio/best"
        archive_file = str(OUTPUT_DIR / "downloaded.txt")
        output_template = str(OUTPUT_DIR / "%(channel)s" / "%(playlist_title|Single Videos)s" / "%(title)s [%(id)s].%(ext)s")
        
        dl_args = [
            "--cookies-from-browser", "firefox",
            "--remote-components", "ejs:github",
            "-f", fmt,
            "--embed-metadata",
            "--embed-thumbnail",
            "--write-subs",
            "--write-auto-subs",
            "--sub-langs", "en.*",
            "--convert-subs", "srt",
            "--download-archive", archive_file,
            "-o", output_template
        ]
        dl_args.extend(sys.argv[1:])
        run_yt_dlp(dl_args)


if __name__ == "__main__":
    main()