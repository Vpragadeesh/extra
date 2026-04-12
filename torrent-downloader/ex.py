
import libtorrent as lt
import time
import sys
from tqdm import tqdm

DOWNLOAD_PATH = "/home/pragadeesh/Videos/"


def human_size(num):
    for unit in ["B", "KB", "MB", "GB", "TB"]:
        if num < 1024:
            return f"{num:.2f} {unit}"
        num /= 1024


def main(torrent_input):
    # ---------------- SESSION ----------------
    ses = lt.session()

    settings = {
        # Unlimited rates
        "download_rate_limit": 0,
        "upload_rate_limit": 0,

        # Aggressive peer usage
        "connections_limit": 2000,
        "unchoke_slots_limit": 200,
        "active_downloads": 20,
        "active_seeds": 20,

        # Timeouts (be impatient)
        "request_timeout": 5,
        "peer_timeout": 20,
        "inactivity_timeout": 20,

        # Request depth (VERY IMPORTANT)
        "max_out_request_queue": 5000,
        "max_allowed_in_request_queue": 5000,

        # Socket buffers
        "recv_socket_buffer_size": 4 << 20,
        "send_socket_buffer_size": 4 << 20,
        "send_buffer_low_watermark": 1 << 20,
        "send_buffer_watermark": 4 << 20,
        "send_buffer_watermark_factor": 200,

        # TCP only (often faster, esp. private torrents)
        "enable_outgoing_utp": False,
        "enable_incoming_utp": False,

        # Behave like a real client
        "allow_multiple_connections_per_ip": True,
        "enable_outgoing_tcp": True,
        "enable_incoming_tcp": True,
    }

    ses.apply_settings(settings)

    # DHT (may be ignored by private torrents, but harmless)
    ses.add_dht_router("router.bittorrent.com", 6881)
    ses.add_dht_router("router.utorrent.com", 6881)
    ses.add_dht_router("dht.transmissionbt.com", 6881)
    ses.start_dht()

    # ---------------- TORRENT ----------------
    params = {
        "save_path": DOWNLOAD_PATH,
        "storage_mode": lt.storage_mode_t.storage_mode_sparse,
    }

    if torrent_input.startswith("magnet:"):
        handle = lt.add_magnet_uri(ses, torrent_input, params)
        print("🔗 Magnet added, fetching metadata…")
    else:
        info = lt.torrent_info(torrent_input)
        handle = ses.add_torrent({"ti": info, "save_path": DOWNLOAD_PATH})

    # ---------------- METADATA ----------------
    while not handle.has_metadata():
        time.sleep(1)

    info = handle.get_torrent_info()

    print("\n📦 Torrent Metadata")
    print("Name       :", info.name())
    print("Total Size :", human_size(info.total_size()))
    print("Files:")

    for f in info.files():
        print(f"  - {f.path} ({human_size(f.size)})")

    # ---------------- CRITICAL FIXES ----------------

    # 1️⃣ Prioritize ALL files (mandatory for repacks)
    handle.prioritize_files([7] * info.num_files())

    # 2️⃣ Allow many peers for this torrent
    handle.set_max_connections(1000)
    handle.set_max_uploads(1000)

    # 3️⃣ DO NOT use sequential download for multi-file torrents
    # (Flud disables it internally for repacks)
    handle.set_sequential_download(False)

    # 4️⃣ Force tracker announces (private torrent friendly)
    handle.force_reannounce()
    handle.force_reannounce(0, 0)

    handle.resume()

    # ---------------- PROGRESS ----------------
    total_size = info.total_size()

    pbar = tqdm(
        total=total_size,
        unit="B",
        unit_scale=True,
        dynamic_ncols=True,
        desc="Downloading",
    )

    last_done = 0

    while not handle.is_seed():
        s = handle.status()

        done = s.total_done
        delta = done - last_done
        last_done = done

        if delta > 0:
            pbar.update(delta)

        pbar.set_postfix(
            speed=f"{human_size(s.download_rate)}/s",
            peers=s.num_peers,
            seeds=s.num_seeds,
        )

        # If tracker is slow, poke it again
        if s.num_peers < 3:
            handle.force_reannounce()

        time.sleep(1)

    pbar.close()
    print("\n✅ Download complete. Seeding 🌱")

    while True:
        time.sleep(60)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python ex.py <magnet-link | torrent-file>")
        sys.exit(1)

    main(sys.argv[1])
