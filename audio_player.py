#!/usr/bin/env python3
"""
Audio player CLI. Play mp3, webp, webm, mp4, mkv, and more.
Extracts audio stream and pipes to mpv for playback.
"""

import os
import sys
import subprocess
import argparse
from pathlib import Path
from typing import Optional


class AudioPlayer:
    """CLI audio player using FFmpeg + mpv."""

    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        self._check_deps()

    def _check_deps(self):
        """Verify ffmpeg and mpv installed."""
        deps = ["ffmpeg", "mpv"]
        missing = []
        for dep in deps:
            if subprocess.run(
                ["which", dep],
                capture_output=True,
                text=True
            ).returncode != 0:
                missing.append(dep)

        if missing:
            print(f"Error: Missing {', '.join(missing)}", file=sys.stderr)
            print("Install: sudo pacman -S ffmpeg mpv  # Arch", file=sys.stderr)
            print("         sudo apt install ffmpeg mpv  # Debian/Ubuntu", file=sys.stderr)
            sys.exit(1)

    def play(self, filepath: str) -> int:
        """Play audio from file. Return exit code."""
        path = Path(filepath).resolve()

        if not path.exists():
            print(f"Error: File not found: {filepath}", file=sys.stderr)
            return 1

        if self.verbose:
            print(f"[*] Playing: {path}", file=sys.stderr)

        try:
            # FFmpeg → mpv pipe. Extract audio only.
            ffmpeg_cmd = [
                "ffmpeg",
                "-i", str(path),
                "-vn",  # No video
                "-acodec", "pcm_s16le",  # PCM audio
                "-ar", "44100",  # 44.1kHz
                "-ac", "2",  # Stereo
                "-f", "wav",
                "-"  # Stdout
            ]

            mpv_cmd = [
                "mpv",
                "--no-video",
                "--audio-display=no",
                "--demuxer-max-bytes=1M",
                "-"  # Stdin
            ]

            if self.verbose:
                print(f"[*] FFmpeg: {' '.join(ffmpeg_cmd)}", file=sys.stderr)

            # Suppress FFmpeg stderr noise unless verbose
            ffmpeg_stderr = None if self.verbose else subprocess.DEVNULL

            ffmpeg_proc = subprocess.Popen(
                ffmpeg_cmd,
                stdout=subprocess.PIPE,
                stderr=ffmpeg_stderr,
                bufsize=65536
            )

            mpv_proc = subprocess.Popen(
                mpv_cmd,
                stdin=ffmpeg_proc.stdout,
                stderr=subprocess.DEVNULL
            )

            # Close FFmpeg stdout in parent (mpv holds reference)
            ffmpeg_proc.stdout.close()

            # Wait for mpv (user stops playback)
            mpv_returncode = mpv_proc.wait()
            ffmpeg_proc.terminate()

            return mpv_returncode

        except KeyboardInterrupt:
            print("\nStopped.", file=sys.stderr)
            ffmpeg_proc.terminate()
            mpv_proc.terminate()
            return 130
        except Exception as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1

    def list_formats(self) -> None:
        """Show supported formats."""
        formats = [
            "Audio: mp3, wav, flac, aac, opus, vorbis, m4a",
            "Video (audio extract): mp4, mkv, webm, avi, mov, flv, m3u8",
            "Others: webp (audio), ogg, wma, alac"
        ]
        for fmt in formats:
            print(fmt)


def main():
    parser = argparse.ArgumentParser(
        description="CLI audio player. Stream audio from any format.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  audio_player.py song.mp3
  audio_player.py video.mp4 --verbose
  audio_player.py --list-formats
        """
    )

    parser.add_argument(
        "file",
        nargs="?",
        help="Audio/video file to play"
    )
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="Show FFmpeg/mpv commands"
    )
    parser.add_argument(
        "--list-formats",
        action="store_true",
        help="Show supported formats"
    )

    args = parser.parse_args()

    player = AudioPlayer(verbose=args.verbose)

    if args.list_formats:
        player.list_formats()
        return 0

    if not args.file:
        parser.print_help()
        return 1

    return player.play(args.file)


if __name__ == "__main__":
    sys.exit(main())
