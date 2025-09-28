import os
import sys
import subprocess
import json
import time
import random
import signal
import select
import tty
import termios
import socket
from pathlib import Path

# --- Configuration ---
MUSIC_ROOT = Path(".")
LIKES_FILE = MUSIC_ROOT / "likes.txt"
DISLIKES_FILE = MUSIC_ROOT / "dislikes.txt"
MPV_SOCKET = Path("/tmp/mpv-musicplayer")
SUPPORTED_EXTENSIONS = ['.mp3', '.wav', '.flac', '.aac', '.ogg', '.m4a']

# --- Globals ---
mpv_process = None
current_folder = ""
songs = []
current_song_index = 0
needs_full_redraw = True
old_termios_settings = None

# --- Cleanup ---
def cleanup():
    """Clean up resources on exit."""
    global mpv_process
    if mpv_process and mpv_process.poll() is None:
        mpv_process.terminate()
        try:
            mpv_process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            mpv_process.kill()
    if MPV_SOCKET.exists():
        MPV_SOCKET.unlink()
    # Restore terminal state
    show_cursor()
    os.system('clear')
    print("Goodbye!")
    # Restore terminal settings
    if old_termios_settings and sys.stdin.isatty():
        termios.tcsetattr(sys.stdin.fileno(), termios.TCSADRAIN, old_termios_settings)
    sys.exit(0)

def signal_handler(sig, frame):
    cleanup()

# --- External Tools ---
def run_fzf(input_list, prompt):
    """Run fzf to select an item from a list."""
    try:
        fzf_proc = subprocess.run(
            ['fzf', '--prompt', prompt, '--height=40%', '--no-sort'],
            input="\n".join(input_list),
            text=True,
            capture_output=True
        )
        return fzf_proc.stdout.strip() if fzf_proc.returncode == 0 else None
    except FileNotFoundError:
        print("Error: 'fzf' command not found. Please install fzf.")
        cleanup()
        sys.exit(1)


# --- Folder/Song Management ---
def select_folder():
    """Interactively select a music folder."""
    global current_folder
    folders = sorted([d.name for d in MUSIC_ROOT.iterdir() if d.is_dir() and not d.name.startswith('.')])
    if not folders:
        print("No music folders found in the current directory.")
        sys.exit(1)

    # Default folder logic
    if "Unni_Menon" in folders:
        selected = "Unni_Menon"
    else:
        print("Default folder 'Unni_Menon' not found. Please select a folder:")
        selected = run_fzf(folders, "🎵 Select Folder: ")

    while not selected:
        print("No folder selected.")
        selected = run_fzf(folders, "🎵 Select Folder: ")
        if not selected:
             if input("No folder selected. Quit? (y/n) ").lower() == 'y':
                cleanup()

    current_folder = selected

def get_songs():
    """Get all supported songs from the current folder."""
    global songs
    folder_path = MUSIC_ROOT / current_folder
    found_songs = []
    for ext in SUPPORTED_EXTENSIONS:
        found_songs.extend(folder_path.glob(f"*{ext}"))
        found_songs.extend(folder_path.glob(f"*{ext.upper()}"))

    songs = sorted(list(set(found_songs))) # Use set to remove duplicates

    if not songs:
        print(f"No songs found in folder '{current_folder}'")
        sys.exit(1)

def choose_song():
    """Interactively choose a song from the current list."""
    global current_song_index
    song_names = [s.name for s in songs]
    selected_song_name = run_fzf(song_names, "🎵 Select Song: ")
    if selected_song_name:
        try:
            current_song_index = song_names.index(selected_song_name)
            return True
        except ValueError:
            return False
    return False

# --- MPV Player Control ---
def play_song():
    """Start playing the current song with mpv."""
    global mpv_process, needs_full_redraw

    if mpv_process and mpv_process.poll() is None:
        mpv_process.terminate()
        try:
            mpv_process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            mpv_process.kill()

    if MPV_SOCKET.exists():
        MPV_SOCKET.unlink()

    song_path = songs[current_song_index]

    # Find MPRIS plugin
    mpris_arg = []
    mpris_candidates = [
        Path.home() / ".config/mpv/scripts/mpris.so",
        Path("/usr/share/mpv/scripts/mpris.so"),
        Path("/usr/local/share/mpv/scripts/mpris.so")
    ]
    for p in mpris_candidates:
        if p.is_file():
            mpris_arg = ["--script", str(p)]
            break

    command = [
        'mpv',
        '--no-video',
        '--quiet',
        '--input-terminal=no',
        f'--input-ipc-server={MPV_SOCKET}',
        *mpris_arg,
        str(song_path)
    ]

    mpv_process = subprocess.Popen(
        command,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    time.sleep(1) # Wait for mpv to start and create the socket
    needs_full_redraw = True

def send_mpv_command(command):
    """Send a command to the mpv IPC socket."""
    if not MPV_SOCKET.exists():
        return None
    try:
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
            s.settimeout(0.5)
            s.connect(str(MPV_SOCKET))
            s.sendall(json.dumps(command).encode('utf-8') + b'\n')
            response_data = s.recv(4096)
            if response_data:
                return json.loads(response_data)
            return None
    except (ConnectionRefusedError, FileNotFoundError, BrokenPipeError, socket.timeout):
        return None

def get_mpv_property(prop):
    """Get a property from mpv."""
    response = send_mpv_command({"command": ["get_property", prop]})
    if response and response.get("error") == "success":
        return response.get("data")
    return None

# --- Information Gathering ---
def get_song_info():
    """Get technical info for the current song using ffprobe."""
    song_path = songs[current_song_index]
    if not song_path.is_file():
        return "- MB", "- kHz", "- kbps", "-"

    try:
        # Get size
        size_bytes = song_path.stat().st_size
        size_mb = f"{size_bytes / (1024*1024):.2f} MB"

        # Get stream info
        probe_cmd = [
            'ffprobe', '-v', 'error', '-select_streams', 'a:0',
            '-show_entries', 'stream=sample_rate,bit_rate,bits_per_raw_sample',
            '-of', 'json', str(song_path)
        ]
        result = subprocess.run(probe_cmd, capture_output=True, text=True, check=True)
        stream_info = json.loads(result.stdout)['streams'][0]

        sample_rate = stream_info.get('sample_rate', 'N/A')
        bit_rate = stream_info.get('bit_rate', 'N/A')
        bit_depth = stream_info.get('bits_per_raw_sample', '0')

        # Format for display
        sample_rate_str = f"{int(sample_rate)//1000} kHz" if sample_rate.isdigit() else "-"
        bit_rate_str = f"{int(bit_rate)//1000} kbps" if bit_rate.isdigit() else "-"
        bit_depth_str = f"{bit_depth}-bit" if str(bit_depth).isdigit() and int(bit_depth) > 0 else "-"

        return size_mb, sample_rate_str, bit_rate_str, bit_depth_str

    except (subprocess.CalledProcessError, json.JSONDecodeError, IndexError, KeyError):
        return f"{song_path.stat().st_size / (1024*1024):.2f} MB", "- kHz", "- kbps", "-"


# --- UI Drawing ---
def format_time_str(seconds):
    """Format seconds to MM:SS."""
    if seconds is None:
        seconds = 0
    try:
        mins, secs = divmod(int(float(seconds)), 60)
        return f"{mins:02d}:{secs:02d}"
    except (ValueError, TypeError):
        return "00:00"


def draw_progress_bar(percent):
    """Draw a Unicode progress bar."""
    percent = int(float(percent or 0))
    width = 50

    filled_len = int(width * percent / 100)
    bar = "█" * filled_len
    empty_len = width - filled_len
    bar += "░" * empty_len
    
    return f"[\033[1;36m{bar}\033[0m] {percent}%"


def update_progress_display(percent, current_pos, total_duration):
    """Update only the progress bar at the bottom of the screen."""
    total_lines = 5 + len(songs) + 5
    
    sys.stdout.write(f"\033[s\033[{total_lines};1H")
    
    if percent is not None and float(percent or 0) > 0:
        progress_bar = draw_progress_bar(percent)
        current_formatted = format_time_str(current_pos)
        total_formatted = format_time_str(total_duration)
        
        line = f"{progress_bar} \033[1;37m{current_formatted} / {total_formatted}\033[0m"
    else:
        line = "\033[1;33mLoading...\033[0m"
        
    sys.stdout.write("\033[K" + line)
    sys.stdout.write("\033[u")
    sys.stdout.flush()

def draw_full_interface():
    """Draw the complete TUI."""
    size, sample_rate, bit_rate, bit_depth = get_song_info()
    
    os.system('clear')
    print("\033[1;35m╔═══════════════════ NOW PLAYING ═══════════════════╗\033[0m")
    print(f"\033[1;36m  {current_folder}/\033[1;33m{songs[current_song_index].name}\033[0m")
    print("\033[1;35m╠═══════════════════════════════════════════════════╣\033[0m")
    print(f"  \033[1;34mSize:\033[0m {size:<10} \033[1;34mSample Rate:\033[0m {sample_rate:<10} \033[1;34mBitrate:\033[0m {bit_rate:<10} \033[1;34mBit Depth:\033[0m {bit_depth}")
    print("\033[1;35m╠═══════════════════════════════════════════════════╣\033[0m")
    
    for i, song in enumerate(songs):
        if i == current_song_index:
            print(f" \033[1;32m▶ {song.name}\033[0m")
        else:
            print(f"  {song.name}")
            
    print("\033[1;35m╠═══════════════════════ CONTROLS ══════════════════════╣\033[0m")
    print("  [\033[1;33ml\033[0m] like    [\033[1;31md\033[0m] dislike    [\033[1mn\033[0m] next")
    print("  [\033[1mb\033[0m] skip −5s    [\033[1mf\033[0m] skip +5s    [\033[1mp/SPACE\033[0m] play/pause")
    print("  [\033[1ms\033[0m] choose song    [\033[1mc\033[0m] change folder    [\033[1mq\033[0m] quit")
    print("\033[1;35m╚═══════════════════════════════════════════════════╝\033[0m")
    print() # Empty line for progress bar
    sys.stdout.flush()

def show_feedback(message, color_code, duration=1):
    """Show a temporary feedback message."""
    total_lines = 5 + len(songs) + 4
    sys.stdout.write(f"\033[s\033[{total_lines};1H\033[K\033[{color_code}m{message}\033[0m\033[u")
    sys.stdout.flush()
    time.sleep(duration)
    sys.stdout.write(f"\033[s\033[{total_lines};1H\033[K\033[u")
    sys.stdout.flush()

# --- Input Handling ---
def get_key():
    """Get a single key press without blocking."""
    if select.select([sys.stdin], [], [], 0) == ([sys.stdin], [], []):
        return sys.stdin.read(1)
    return None

def hide_cursor():
    sys.stdout.write("\033[?25l")
    sys.stdout.flush()

def show_cursor():
    sys.stdout.write("\033[?25h")
    sys.stdout.flush()

# --- Main Application Logic ---
def main():
    global needs_full_redraw, current_song_index, old_termios_settings

    if not sys.stdin.isatty():
        print("This script requires an interactive terminal.")
        sys.exit(1)

    old_termios_settings = termios.tcgetattr(sys.stdin)
    tty.setcbreak(sys.stdin.fileno())

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    select_folder()
    get_songs()
    
    current_song_index = random.randint(0, len(songs) - 1)
    play_song()
    
    hide_cursor()
    
    last_progress_update = 0

    try:
        while True:
            current_time = time.time()

            if needs_full_redraw:
                draw_full_interface()
                needs_full_redraw = False
                last_progress_update = 0

            if current_time - last_progress_update >= 1:
                percent = get_mpv_property("percent-pos")
                pos = get_mpv_property("time-pos")
                dur = get_mpv_property("duration")
                update_progress_display(percent, pos, dur)
                last_progress_update = current_time

            if mpv_process and mpv_process.poll() is not None:
                current_song_index = (current_song_index + 1) % len(songs)
                play_song()
                continue

            key = get_key()
            if key:
                if key in ('l', 'L'):
                    with open(LIKES_FILE, "a") as f:
                        f.write(f"{current_folder}/{songs[current_song_index].name}\n")
                    show_feedback("Liked!", "1;32")
                elif key in ('d', 'D'):
                    with open(DISLIKES_FILE, "a") as f:
                        f.write(f"{current_folder}/{songs[current_song_index].name}\n")
                    show_feedback("Disliked!", "1;31")
                elif key in ('n', 'N'):
                    current_song_index = (current_song_index + 1) % len(songs)
                    play_song()
                elif key in ('s', 'S'):
                    if choose_song():
                        play_song()
                    else: # Redraw if fzf was cancelled
                        needs_full_redraw = True
                elif key in ('c', 'C'):
                    select_folder()
                    get_songs()
                    current_song_index = 0
                    play_song()
                elif key in ('q', 'Q'):
                    cleanup()
                elif key in ('p', 'P', ' '):
                    send_mpv_command({"command": ["cycle", "pause"]})
                elif key in ('b', 'B'):
                    send_mpv_command({"command": ["seek", -5, "relative"]})
                elif key in ('f', 'F'):
                    send_mpv_command({"command": ["seek", 5, "relative"]})
                
                if key not in ('p', 'P', ' ', 'b', 'B', 'f', 'F', 'l', 'L', 'd', 'D'):
                     needs_full_redraw = True

            time.sleep(0.1)
    finally:
        cleanup()

if __name__ == "__main__":
    for cmd in ['mpv', 'fzf', 'ffprobe']:
        if subprocess.run(['which', cmd], capture_output=True).returncode != 0:
            print(f"Error: Required command '{cmd}' not found in PATH.")
            sys.exit(1)
            
    main()
