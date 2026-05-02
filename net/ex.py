import shutil
import tempfile
from pathlib import Path

link = 'https://www.youtube.com/watch?v=bYHJ7Q0qAsc'

try:
    from yt_dlp import YoutubeDL
except Exception:
    print("yt-dlp is not installed. Install it with: pip install yt-dlp")
    raise


def download_video(link: str, cleanup: bool = True, cookies: str | None = None, user_agent: str | None = None) -> None:
    """Download a YouTube video into a temporary folder and delete it at the end.

    Args:
        link: The YouTube video URL.
        cleanup: If True, delete the downloaded file and folder when done.
        cookies: Optional path to a cookies.txt file exported from your browser (helps with age-restricted or signed-in-only videos).
        user_agent: Optional custom User-Agent string to use for HTTP requests.
    """

    # Create a temporary directory for the download
    tmp_dir = Path(tempfile.mkdtemp(prefix="yt_download_"))
    print(f"Created temporary folder: {tmp_dir}")

    # Default to a modern browser User-Agent to avoid 403s
    default_ua = (
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/120.0.0.0 Safari/537.36"
    )
    headers = {
        'User-Agent': user_agent or default_ua,
        'Referer': 'https://www.youtube.com/',
    }

    ydl_opts = {
        'outtmpl': str(tmp_dir / '%(title)s.%(ext)s'),
        'format': 'bestvideo+bestaudio/best',
        'noplaylist': True,
        'quiet': False,
        'no_warnings': True,
        'http_headers': headers,
    }

    if cookies:
        ydl_opts['cookiefile'] = str(cookies)

    def _do_download(opts):
        with YoutubeDL(opts) as ydl:
            info = ydl.extract_info(link, download=True)
            try:
                return ydl.prepare_filename(info)
            except Exception:
                return str(tmp_dir / (info.get('title', 'video') + '.mp4'))

    try:
        filename = _do_download(ydl_opts)
        print(f"Download completed: {filename}")

    except Exception as e:
        err = str(e)
        print(f"An error occurred during download: {err}")

        # Common fix for YouTube 403 errors: retry using a browser UA and suggest cookies
        if '403' in err or 'HTTP Error 403' in err:
            print("Received 403 Forbidden — retrying once with a browser User-Agent and Referer...")
            ydl_opts_retry = dict(ydl_opts)
            ydl_opts_retry['http_headers'] = {
                'User-Agent': default_ua,
                'Referer': 'https://www.youtube.com/',
            }
            try:
                filename = _do_download(ydl_opts_retry)
                print(f"Download completed on retry: {filename}")
            except Exception as e2:
                print(f"Retry failed: {e2}")
                print(
                    "If this is an age-restricted or region-restricted video, try exporting cookies from your browser"
                    " (see https://github.com/yt-dlp/yt-dlp#cookies) and pass the path via the 'cookies' argument."
                )

    finally:
        if cleanup:
            # Remove the temporary directory and all its contents
            try:
                shutil.rmtree(tmp_dir)
                print(f"Deleted temporary folder: {tmp_dir}")
            except Exception as e:
                print(f"Failed to delete temporary folder {tmp_dir}: {e}")


if __name__ == "__main__":
    download_video(link)
