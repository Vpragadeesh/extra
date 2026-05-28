link='https://www.youtube.com/watch?v=bYHJ7Q0qAsc'
from pytube import Youtube

def download_video(link):
    try:
        yt = Youtube(link)
        stream = yt.streams.get_highest_resolution()
        print(f"Downloading: {yt.title}")
        stream.download()
        print("Download completed!")
        print("Download completed!")
    except Exception as e:
        print(f"An error occurred: {e}")
download_video(link)
