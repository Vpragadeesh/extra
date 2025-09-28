#!/usr/bin/env python3
"""
Terminal-based Video Player with FZF Integration
Opens videos in VLC from an attractive terminal interface
"""

import os
import sys
import subprocess
import glob
import shutil
from pathlib import Path
import json
import time
import argparse

class TerminalVideoPlayer:
    def __init__(self, start_directory=None, auto_play=None, player_cmd=None, recursive=True, show_hidden=False):
        self.current_dir = os.path.abspath(start_directory) if start_directory else os.getcwd()
        self.video_extensions = ['.mp4', '.avi', '.mkv', '.mov', '.wmv', '.flv', '.webm', '.m4v', '.mp3', '.wav', '.flac']
        self.auto_play = auto_play
        self.custom_player = player_cmd
        self.recursive = recursive
        self.show_hidden = show_hidden
        self.colors = {
            'red': '\033[91m',
            'green': '\033[92m',
            'yellow': '\033[93m',
            'blue': '\033[94m',
            'magenta': '\033[95m',
            'cyan': '\033[96m',
            'white': '\033[97m',
            'bold': '\033[1m',
            'underline': '\033[4m',
            'end': '\033[0m',
            'bg_blue': '\033[44m',
            'bg_green': '\033[42m'
        }
        self.use_fzf = self.check_fzf_available()
        
    def check_fzf_available(self):
        """Check if fzf is available on the system"""
        return shutil.which('fzf') is not None
    
    def colorize(self, text, color):
        """Add color to text"""
        return f"{self.colors.get(color, '')}{text}{self.colors['end']}"
    
    def print_header(self):
        """Print an attractive header"""
        width = 70
        print("\n" + "═" * width)
        print(self.colorize("🎬 TERMINAL VIDEO PLAYER 🎬", 'bold') + self.colorize(" v2.0", 'cyan'))
        print("═" * width)
        print(self.colorize(f"📁 Directory: {self.current_dir}", 'yellow'))
        if self.use_fzf:
            print(self.colorize("⚡ FZF Mode: Enabled", 'green'))
        else:
            print(self.colorize("📋 FZF Mode: Disabled (install fzf for better experience)", 'yellow'))
        print("═" * width)
    
    def print_footer(self):
        """Print an attractive footer"""
        width = 70
        print("═" * width)
        print(self.colorize("✨ Happy watching! ✨", 'magenta'))
        print("═" * width)
        
    def find_videos(self, directory=None):
        """Find all video files in the current directory and optionally subdirectories"""
        if directory is None:
            directory = self.current_dir
            
        videos = []
        
        if self.recursive:
            # Search recursively for video files
            for root, dirs, files in os.walk(directory):
                # Skip hidden directories if not showing hidden files
                if not self.show_hidden:
                    dirs[:] = [d for d in dirs if not d.startswith('.')]
                
                for file in files:
                    # Skip hidden files if not showing hidden files
                    if not self.show_hidden and file.startswith('.'):
                        continue
                        
                    file_lower = file.lower()
                    if any(file_lower.endswith(ext) for ext in self.video_extensions):
                        full_path = os.path.join(root, file)
                        videos.append(full_path)
        else:
            # Search only in current directory
            for file in os.listdir(directory):
                if not self.show_hidden and file.startswith('.'):
                    continue
                    
                file_path = os.path.join(directory, file)
                if os.path.isfile(file_path):
                    file_lower = file.lower()
                    if any(file_lower.endswith(ext) for ext in self.video_extensions):
                        videos.append(file_path)
        
        # Sort videos alphabetically
        videos.sort()
        return videos
    
    def format_video_info(self, video_path):
        """Get formatted video information"""
        filename = os.path.basename(video_path)
        rel_path = os.path.relpath(video_path, self.current_dir)
        
        # Get file size
        try:
            size_bytes = os.path.getsize(video_path)
            if size_bytes > 1024**3:  # GB
                size = f"{size_bytes / (1024**3):.1f} GB"
            elif size_bytes > 1024**2:  # MB
                size = f"{size_bytes / (1024**2):.1f} MB"
            else:  # KB
                size = f"{size_bytes / 1024:.1f} KB"
        except:
            size = "Unknown"
        
        # Get file extension
        ext = os.path.splitext(filename)[1].upper()
        
        return {
            'filename': filename,
            'path': rel_path,
            'size': size,
            'ext': ext,
            'full_path': video_path
        }
    
    def fzf_select_video(self, videos):
        """Use fzf to select a video file"""
        if not videos:
            return None
            
        # Prepare video list for fzf
        video_items = []
        for video in videos:
            info = self.format_video_info(video)
            # Create a nice display format
            display_line = f"{info['filename']} [{info['ext']}] [{info['size']}] ({info['path']})"
            video_items.append(display_line)
        
        try:
            # Create fzf command with nice preview and options
            fzf_cmd = [
                'fzf',
                '--prompt=🎥 Select video: ',
                '--height=60%',
                '--reverse',
                '--border',
                '--info=inline',
                '--color=fg:#f8f8f2,bg:#282a36,hl:#bd93f9',
                '--color=fg+:#f8f8f2,bg+:#44475a,hl+:#bd93f9',
                '--color=info:#ffb86c,prompt:#50fa7b,pointer:#ff79c6',
                '--color=marker:#ff79c6,spinner:#ffb86c,header:#6272a4',
                '--header=📁 Use ↑↓ to navigate, Enter to select, Esc to cancel',
                '--preview-window=right:30%',
            ]
            
            # Run fzf
            process = subprocess.Popen(
                fzf_cmd,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            
            # Send video list to fzf
            input_text = '\n'.join(video_items)
            stdout, stderr = process.communicate(input=input_text)
            
            if process.returncode == 0 and stdout.strip():
                # Find the selected video by matching the display line
                selected_line = stdout.strip()
                for i, item in enumerate(video_items):
                    if item == selected_line:
                        return videos[i]
            
            return None
            
        except Exception as e:
            print(self.colorize(f"❌ Error with fzf: {e}", 'red'))
            return None
    
    def display_videos(self, videos):
        """Display the list of videos with enhanced formatting"""
        if not videos:
            print(self.colorize("\n❌ No video files found in the current directory.", 'red'))
            print(self.colorize("💡 Tip: This player searches recursively in subdirectories too!", 'cyan'))
            return False
        
        print(f"\n{self.colorize('📊 Found', 'green')} {self.colorize(str(len(videos)), 'bold')} {self.colorize('video file(s)', 'green')}")
        print("─" * 70)
        
        if self.use_fzf:
            print(self.colorize("⚡ FZF mode enabled - enhanced selection available!", 'cyan'))
        else:
            # Display first 10 videos in list mode
            display_count = min(10, len(videos))
            for i in range(display_count):
                video = videos[i]
                info = self.format_video_info(video)
                
                # Create a nice display format with colors
                number = self.colorize(f"{i+1:2d}.", 'yellow')
                name = self.colorize(info['filename'], 'white')
                ext = self.colorize(f"[{info['ext']}]", 'magenta')
                size = self.colorize(f"[{info['size']}]", 'cyan')
                
                print(f"{number} {name} {ext} {size}")
                
                # Truncate if filename too long
                if len(info['filename']) > 50:
                    print(f"    {self.colorize('📁 ' + info['path'], 'blue')}")
            
            if len(videos) > 10:
                remaining = len(videos) - 10
                print(self.colorize(f"    ... and {remaining} more files", 'yellow'))
                print(self.colorize("    💡 Install 'fzf' for better browsing experience!", 'cyan'))
        
        print("─" * 70)
        return True
    
    def show_menu(self, video_count):
        """Display the main menu with attractive formatting"""
        print(f"\n{self.colorize('🎯 OPTIONS:', 'bold')}")
        
        if self.use_fzf:
            print(f"  {self.colorize('f', 'green')} - {self.colorize('Use FZF to select video', 'white')}")
        
        if video_count <= 20:  # Show numbered selection for small lists
            print(f"  {self.colorize('1-' + str(video_count), 'yellow')} - {self.colorize('Select video by number', 'white')}")
        
        print(f"  {self.colorize('l', 'blue')} - {self.colorize('List all videos', 'white')}")
        print(f"  {self.colorize('r', 'magenta')} - {self.colorize('Refresh/Rescan directory', 'white')}")
        print(f"  {self.colorize('d', 'cyan')} - {self.colorize('Change directory', 'white')}")
        print(f"  {self.colorize('q', 'red')} - {self.colorize('Quit', 'white')}")
        print("─" * 40)
    
    def get_user_choice(self, videos):
        """Get user's selection with enhanced interface"""
        while True:
            try:
                self.show_menu(len(videos))
                
                choice = input(f"\n{self.colorize('🚀 Your choice: ', 'bold')}").strip().lower()
                
                if choice == 'q':
                    return 'quit'
                elif choice == 'r':
                    return 'refresh'
                elif choice == 'l':
                    return 'list'
                elif choice == 'd':
                    return 'change_dir'
                elif choice == 'f' and self.use_fzf:
                    selected = self.fzf_select_video(videos)
                    if selected:
                        return ('play', selected)
                    else:
                        print(self.colorize("❌ No video selected.", 'yellow'))
                        continue
                else:
                    try:
                        num = int(choice)
                        if 1 <= num <= len(videos):
                            return ('play', videos[num - 1])
                        else:
                            print(self.colorize(f"❌ Please enter a number between 1 and {len(videos)}", 'red'))
                    except ValueError:
                        print(self.colorize("❌ Invalid input. Please try again.", 'red'))
                        
            except KeyboardInterrupt:
                print(self.colorize("\n👋 Exiting...", 'yellow'))
                return 'quit'
    
    def change_directory(self):
        """Allow user to change the current directory"""
        print(f"\n{self.colorize('📁 CHANGE DIRECTORY', 'bold')}")
        print(f"Current: {self.colorize(self.current_dir, 'cyan')}")
        
        new_dir = input(f"\nEnter new directory path (or press Enter to cancel): ").strip()
        
        if not new_dir:
            return False
            
        if new_dir.startswith('~'):
            new_dir = os.path.expanduser(new_dir)
            
        if os.path.isdir(new_dir):
            self.current_dir = os.path.abspath(new_dir)
            print(self.colorize(f"✅ Changed to: {self.current_dir}", 'green'))
            return True
        else:
            print(self.colorize(f"❌ Directory not found: {new_dir}", 'red'))
            return False
    
    def list_all_videos(self, videos):
        """List all videos with detailed information"""
        if not videos:
            print(self.colorize("❌ No videos found.", 'red'))
            return
            
        print(f"\n{self.colorize('📋 ALL VIDEOS', 'bold')} ({len(videos)} files)")
        print("═" * 80)
        
        for i, video in enumerate(videos, 1):
            info = self.format_video_info(video)
            
            # Format the display line
            number = self.colorize(f"{i:3d}.", 'yellow')
            name = self.colorize(info['filename'], 'white')
            ext = self.colorize(f"[{info['ext']}]", 'magenta')
            size = self.colorize(f"[{info['size']}]", 'cyan')
            path = self.colorize(info['path'], 'blue')
            
            print(f"{number} {name} {ext} {size}")
            if info['path'] != info['filename']:  # Show path if different from filename
                print(f"     📁 {path}")
            
        print("═" * 80)
        input(f"\n{self.colorize('Press Enter to continue...', 'yellow')}")
    
    def play_video(self, video_path):
        """Play video using configured player with enhanced feedback"""
        info = self.format_video_info(video_path)
        
        print(f"\n{self.colorize('🎥 PLAYING VIDEO', 'bold')}")
        print("─" * 50)
        print(f"📽️  File: {self.colorize(info['filename'], 'cyan')}")
        print(f"📁 Path: {self.colorize(info['path'], 'blue')}")
        print(f"📊 Size: {self.colorize(info['size'], 'yellow')}")
        print(f"🎬 Type: {self.colorize(info['ext'], 'magenta')}")
        print("─" * 50)
        
        # Animation while opening (only in interactive mode)
        if not self.auto_play:
            print("🚀 Launching player", end="", flush=True)
            for i in range(3):
                time.sleep(0.5)
                print(".", end="", flush=True)
            print()
        
        try:
            # Use custom player if specified
            if self.custom_player:
                player_commands = [self.custom_player]
            else:
                # Try different player commands based on system
                player_commands = ['vlc', '/usr/bin/vlc', '/snap/bin/vlc', '/Applications/VLC.app/Contents/MacOS/VLC', 'mpv', 'mplayer']
            
            for cmd in player_commands:
                try:
                    # Run player in the background so terminal remains usable
                    subprocess.Popen([cmd, video_path], 
                                   stdout=subprocess.DEVNULL, 
                                   stderr=subprocess.DEVNULL)
                    player_name = os.path.basename(cmd).upper()
                    print(self.colorize(f"✅ {player_name} opened successfully!", 'green'))
                    if not self.auto_play:
                        print(self.colorize("💡 You can continue using this player while the video is running.", 'cyan'))
                    return True
                except FileNotFoundError:
                    continue
            
            print(self.colorize("❌ No video player found. Please install VLC, MPV, or specify a custom player.", 'red'))
            print(self.colorize("💡 Try: sudo apt install vlc (Ubuntu/Debian) or brew install vlc (macOS)", 'yellow'))
            return False
            
        except Exception as e:
            print(self.colorize(f"❌ Error opening video: {e}", 'red'))
            return False
    
    def run(self):
        """Main program loop with enhanced interface"""
        # Clear screen for better presentation
        os.system('clear' if os.name == 'posix' else 'cls')
        
        self.print_header()
        
        if not self.use_fzf:
            print(self.colorize("\n💡 Install fzf for enhanced video selection experience!", 'yellow'))
            print(self.colorize("   sudo apt install fzf  # Ubuntu/Debian", 'blue'))
            print(self.colorize("   brew install fzf      # macOS", 'blue'))
        
        while True:
            print(f"\n{self.colorize('🔍 Scanning for videos...', 'yellow')}")
            videos = self.find_videos()
            
            if not self.display_videos(videos):
                print(f"\n{self.colorize('💡 Tips:', 'cyan')}")
                print("   • Make sure you're in the right directory")
                print("   • Supported formats: MP4, AVI, MKV, MOV, WMV, FLV, WebM, M4V")
                print("   • This player searches subdirectories recursively")
                
                retry = input(f"\n{self.colorize('Try a different directory? (y/n): ', 'yellow')}").strip().lower()
                if retry in ['y', 'yes']:
                    if self.change_directory():
                        continue
                break
            
            choice = self.get_user_choice(videos)
            
            if choice == 'quit':
                break
            elif choice == 'refresh':
                print(self.colorize("🔄 Refreshing video list...", 'yellow'))
                continue
            elif choice == 'list':
                self.list_all_videos(videos)
                continue
            elif choice == 'change_dir':
                self.change_directory()
                continue
            elif isinstance(choice, tuple) and choice[0] == 'play':
                video_path = choice[1]
                success = self.play_video(video_path)
                
                if success:
                    # Ask if user wants to continue
                    print("\n" + "─" * 50)
                    cont = input(f"{self.colorize('🎬 Play another video? (y/n): ', 'green')}").strip().lower()
                    if cont not in ['y', 'yes', '']:
                        break
                else:
                    input(f"\n{self.colorize('Press Enter to continue...', 'yellow')}")
        
        self.print_footer()
    
    def list_videos_cli(self, videos):
        """List videos for CLI output (non-interactive)"""
        if not videos:
            print(self.colorize("No video files found.", 'red'))
            return
            
        print(f"\nFound {self.colorize(str(len(videos)), 'green')} video file(s) in {self.colorize(self.current_dir, 'cyan')}")
        print("─" * 80)
        
        for i, video in enumerate(videos, 1):
            info = self.format_video_info(video)
            print(f"{i:3d}. {info['filename']} [{info['ext']}] [{info['size']}]")
            if info['path'] != info['filename']:
                print(f"     📁 {info['path']}")
        
        print("─" * 80)
    
    def play_video_by_number(self, videos, number):
        """Play video by its number in the list"""
        if 1 <= number <= len(videos):
            return self.play_video(videos[number - 1])
        else:
            print(self.colorize(f"❌ Invalid video number. Please choose between 1 and {len(videos)}", 'red'))
            return False
    
    def play_video_by_name(self, videos, name):
        """Play video by matching filename (fuzzy matching)"""
        name_lower = name.lower()
        matches = []
        
        for video in videos:
            filename = os.path.basename(video).lower()
            if name_lower in filename:
                matches.append(video)
        
        if not matches:
            print(self.colorize(f"❌ No video found matching '{name}'", 'red'))
            return False
        elif len(matches) == 1:
            return self.play_video(matches[0])
        else:
            print(self.colorize(f"Multiple matches found for '{name}':", 'yellow'))
            for i, match in enumerate(matches, 1):
                filename = os.path.basename(match)
                print(f"{i}. {filename}")
            print(self.colorize("Please be more specific or use the video number.", 'yellow'))
            return False

def create_parser():
    """Create command line argument parser"""
    parser = argparse.ArgumentParser(
        description='🎬 Terminal Video Player - A beautiful CLI video player with FZF integration',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
Examples:
  %(prog)s                          # Interactive mode (default)
  %(prog)s --list                   # List all videos
  %(prog)s --play 1                 # Play video number 1
  %(prog)s --play "movie.mp4"       # Play specific video
  %(prog)s --directory ~/Videos     # Start in specific directory
  %(prog)s --no-recursive           # Search only current directory
  %(prog)s --player mpv             # Use MPV instead of VLC
  %(prog)s --fzf                    # Force FZF selection
  %(prog)s --auto-play "video.mp4"  # Auto-play and exit
        '''
    )
    
    # Main actions
    action_group = parser.add_mutually_exclusive_group()
    action_group.add_argument(
        '--list', '-l',
        action='store_true',
        help='List all video files and exit'
    )
    action_group.add_argument(
        '--play', '-p',
        metavar='VIDEO',
        help='Play specific video (by number or name) and exit'
    )
    action_group.add_argument(
        '--fzf', '-f',
        action='store_true',
        help='Use FZF to select video and exit'
    )
    action_group.add_argument(
        '--auto-play',
        metavar='VIDEO',
        help='Automatically play video and exit (no interaction)'
    )
    
    # Options
    parser.add_argument(
        '--directory', '-d',
        metavar='DIR',
        help='Start in specific directory (default: current directory)'
    )
    parser.add_argument(
        '--player',
        metavar='CMD',
        help='Use specific video player command (default: auto-detect VLC/MPV)'
    )
    parser.add_argument(
        '--no-recursive', '-nr',
        action='store_true',
        help='Search only in current directory (not subdirectories)'
    )
    parser.add_argument(
        '--show-hidden',
        action='store_true',
        help='Include hidden files and directories in search'
    )
    parser.add_argument(
        '--version', '-v',
        action='version',
        version='Terminal Video Player v2.0'
    )
    
    # Interactive mode options
    parser.add_argument(
        '--no-color',
        action='store_true',
        help='Disable colored output'
    )
    parser.add_argument(
        '--quiet', '-q',
        action='store_true',
        help='Minimal output (useful for scripting)'
    )
    
    return parser

def main():
    """Entry point with CLI argument parsing"""
    parser = create_parser()
    args = parser.parse_args()
    
    try:
        # Initialize player with CLI options
        player = TerminalVideoPlayer(
            start_directory=args.directory,
            auto_play=args.auto_play,
            player_cmd=args.player,
            recursive=not args.no_recursive,
            show_hidden=args.show_hidden
        )
        
        # Disable colors if requested
        if args.no_color:
            player.colors = {key: '' for key in player.colors}
        
        # Handle CLI-specific actions
        if args.list or args.play or args.fzf or args.auto_play:
            # Non-interactive mode
            if not args.quiet:
                if not args.auto_play:  # Don't show header in auto-play mode
                    player.print_header()
            
            videos = player.find_videos()
            
            if args.list:
                player.list_videos_cli(videos)
                
            elif args.play:
                try:
                    # Try to parse as number first
                    video_num = int(args.play)
                    success = player.play_video_by_number(videos, video_num)
                except ValueError:
                    # Treat as filename
                    success = player.play_video_by_name(videos, args.play)
                
                sys.exit(0 if success else 1)
                
            elif args.fzf:
                if not player.use_fzf:
                    print(player.colorize("❌ FZF not available. Please install fzf.", 'red'))
                    sys.exit(1)
                
                selected = player.fzf_select_video(videos)
                if selected:
                    success = player.play_video(selected)
                    sys.exit(0 if success else 1)
                else:
                    print(player.colorize("❌ No video selected.", 'yellow'))
                    sys.exit(1)
                    
            elif args.auto_play:
                # Find video by name and play it
                success = player.play_video_by_name(videos, args.auto_play)
                sys.exit(0 if success else 1)
        else:
            # Interactive mode (default)
            player.run()
            
    except KeyboardInterrupt:
        print(f"\n👋 Goodbye!")
        sys.exit(0)
    except Exception as e:
        if args.quiet if 'args' in locals() else False:
            sys.exit(1)
        else:
            print(f"❌ An error occurred: {e}")
            sys.exit(1)

if __name__ == "__main__":
    main()