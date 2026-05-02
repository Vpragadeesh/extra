#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>
#include <unistd.h>
#include <signal.h>
#include <time.h>
#include <termios.h>
#include <sys/select.h>
#include <curl/curl.h>
#include <stdatomic.h>

// Target: 600GB
#define TARGET_GB 600
#define TARGET_BYTES ((unsigned long long)TARGET_GB * 1024ULL * 1024ULL * 1024ULL)
#define CHUNK_SIZE (512 * 1024)  // 512KB chunks (optimal for network)
#define THREADS_PER_URL 4  // More parallel connections per source

// Passive mode settings
#define DOWNLOAD_DURATION 60  // seconds to download actively
#define SLEEP_DURATION 10  // seconds to sleep between bursts

// Test file URLs
static const char *TEST_FILES[] = {
    "http://speedtest.tele2.net/10GB.zip",
    "http://speedtest.tele2.net/1GB.zip",
    "http://proof.ovh.net/files/10Gb.dat",
    "http://proof.ovh.net/files/1Gb.dat",
    "http://speedtest.belwue.net/100M",
    "http://speedtest-sgp1.digitalocean.com/10gb.test",
    "http://speedtest-ams2.digitalocean.com/10gb.test",
    "http://speedtest.ftp.otenet.gr/files/test10Gb.db",
};
#define NUM_URLS (sizeof(TEST_FILES) / sizeof(TEST_FILES[0]))
#define NUM_THREADS (NUM_URLS * THREADS_PER_URL)

// Shared state
static atomic_ullong total_bytes = 0;
static atomic_int stop_flag = 0;
static atomic_int pause_flag = 0;
static atomic_int passive_sleep_flag = 0;  // For passive mode sleep cycles
static pthread_mutex_t print_lock = PTHREAD_MUTEX_INITIALIZER;

// Terminal settings for keyboard input
static struct termios old_termios;
static int termios_saved = 0;

// Thread argument structure
typedef struct {
    const char *url;
    int thread_id;
} thread_arg_t;

// Format bytes to human readable
static void format_bytes(unsigned long long bytes, char *buf, size_t buflen) {
    const char *units[] = {"B", "KB", "MB", "GB", "TB", "PB"};
    int unit_idx = 0;
    double value = (double)bytes;

    while (value >= 1024.0 && unit_idx < 5) {
        value /= 1024.0;
        unit_idx++;
    }
    snprintf(buf, buflen, "%.2f %s", value, units[unit_idx]);
}

// Format speed in MB/s
static void format_speed(double bytes_per_sec, char *buf, size_t buflen) {
    double mbps = bytes_per_sec / (1024.0 * 1024.0);
    snprintf(buf, buflen, "%.1f MB/s", mbps);
}

// CURL write callback - just count bytes and discard data
static size_t write_callback(void *contents, size_t size, size_t nmemb, void *userp) {
    (void)contents;
    (void)userp;
    size_t real_size = size * nmemb;

    // Wait while paused or in passive sleep
    while ((atomic_load(&pause_flag) || atomic_load(&passive_sleep_flag)) && !atomic_load(&stop_flag)) {
        usleep(100000);  // 100ms
    }

    if (atomic_load(&stop_flag)) {
        return 0;  // Signal curl to stop
    }

    atomic_fetch_add(&total_bytes, real_size);
    return real_size;
}

// Download thread function
static void *download_thread(void *arg) {
    thread_arg_t *targ = (thread_arg_t *)arg;
    CURL *curl;
    CURLcode res;

    curl = curl_easy_init();
    if (!curl) {
        pthread_mutex_lock(&print_lock);
        fprintf(stderr, "Thread %d: Failed to init curl\n", targ->thread_id);
        pthread_mutex_unlock(&print_lock);
        free(targ);
        return NULL;
    }

    while (!atomic_load(&stop_flag)) {
        curl_easy_setopt(curl, CURLOPT_URL, targ->url);
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_callback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, NULL);
        curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 1L);
        curl_easy_setopt(curl, CURLOPT_TIMEOUT, 0L);  // No timeout
        curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT, 10L);  // Faster connection timeout
        curl_easy_setopt(curl, CURLOPT_NOSIGNAL, 1L);
        curl_easy_setopt(curl, CURLOPT_BUFFERSIZE, CHUNK_SIZE);

        // Speed optimizations
        curl_easy_setopt(curl, CURLOPT_TCP_NODELAY, 1L);  // Disable Nagle's algorithm
        curl_easy_setopt(curl, CURLOPT_TCP_KEEPALIVE, 1L);  // Keep connections alive
        curl_easy_setopt(curl, CURLOPT_ACCEPT_ENCODING, "");  // Accept any encoding (gzip, etc.)
        curl_easy_setopt(curl, CURLOPT_HTTP_VERSION, CURL_HTTP_VERSION_1_1);  // Force HTTP/1.1
        curl_easy_setopt(curl, CURLOPT_FORBID_REUSE, 0L);  // Allow connection reuse
        curl_easy_setopt(curl, CURLOPT_FRESH_CONNECT, 0L);  // Use cached connections
        curl_easy_setopt(curl, CURLOPT_DNS_CACHE_TIMEOUT, 3600L);  // Cache DNS for 1 hour
        curl_easy_setopt(curl, CURLOPT_IPRESOLVE, CURL_IPRESOLVE_V4);  // IPv4 only (often faster)

        res = curl_easy_perform(curl);

        if (res != CURLE_OK && !atomic_load(&stop_flag)) {
            // Brief pause on error, then retry
            sleep(1);
        }
    }

    curl_easy_cleanup(curl);
    free(targ);
    return NULL;
}

// Keyboard listener thread
static void *keyboard_listener(void *arg) {
    (void)arg;
    fd_set readfds;
    struct timeval tv;
    char c;

    while (!atomic_load(&stop_flag)) {
        FD_ZERO(&readfds);
        FD_SET(STDIN_FILENO, &readfds);
        tv.tv_sec = 0;
        tv.tv_usec = 100000;  // 100ms

        if (select(STDIN_FILENO + 1, &readfds, NULL, NULL, &tv) > 0) {
            if (read(STDIN_FILENO, &c, 1) == 1) {
                if (c == 'p' || c == 'P') {
                    int paused = !atomic_load(&pause_flag);
                    atomic_store(&pause_flag, paused);

                    pthread_mutex_lock(&print_lock);
                    if (paused) {
                        printf("\n⏸️  PAUSED - Press 'p' to resume\n");
                    } else {
                        printf("\n▶️  RESUMED - Press 'p' to pause\n");
                    }
                    fflush(stdout);
                    pthread_mutex_unlock(&print_lock);
                } else if (c == 'q' || c == 'Q') {
                    pthread_mutex_lock(&print_lock);
                    printf("\n🛑 Stopping downloads...\n");
                    fflush(stdout);
                    pthread_mutex_unlock(&print_lock);
                    atomic_store(&stop_flag, 1);
                    break;
                }
            }
        }
    }
    return NULL;
}

// Passive mode manager thread
static void *passive_mode_manager(void *arg) {
    (void)arg;
    int cycle = 0;

    while (!atomic_load(&stop_flag)) {
        // Download phase
        pthread_mutex_lock(&print_lock);
        printf("\n🟢 Passive Mode: Starting download burst (cycle %d)\n", ++cycle);
        fflush(stdout);
        pthread_mutex_unlock(&print_lock);

        atomic_store(&passive_sleep_flag, 0);
        sleep(DOWNLOAD_DURATION);

        if (atomic_load(&stop_flag)) break;

        // Sleep phase
        pthread_mutex_lock(&print_lock);
        printf("\n💤 Passive Mode: Sleeping for %d seconds...\n", SLEEP_DURATION);
        fflush(stdout);
        pthread_mutex_unlock(&print_lock);

        atomic_store(&passive_sleep_flag, 1);
        sleep(SLEEP_DURATION);
    }

    return NULL;
}

// Format ETA to human readable
static void format_eta(double seconds, char *buf, size_t buflen) {
    if (seconds < 0 || seconds > 365 * 24 * 3600) {
        snprintf(buf, buflen, "--:--:--");
        return;
    }
    int hours = (int)(seconds / 3600);
    int mins = (int)((seconds - hours * 3600) / 60);
    int secs = (int)(seconds - hours * 3600 - mins * 60);
    snprintf(buf, buflen, "%02d:%02d:%02d", hours, mins, secs);
}

// Progress bar
static void print_progress(unsigned long long bytes, unsigned long long total,
                          double speed, double elapsed_hours, double eta_seconds, int paused, int sleeping) {
    char bytes_str[32], total_str[32], speed_str[32], eta_str[32];
    int bar_width = 30;
    double progress = (double)bytes / (double)total;
    int filled = (int)(progress * bar_width);

    format_bytes(bytes, bytes_str, sizeof(bytes_str));
    format_bytes(total, total_str, sizeof(total_str));
    format_speed(speed, speed_str, sizeof(speed_str));
    format_eta(eta_seconds, eta_str, sizeof(eta_str));

    printf("\r");
    if (sleeping) {
        printf("💤 SLEEPING: ");
    } else if (paused) {
        printf("⏸️  PAUSED : ");
    } else {
        printf("📥 Progress: ");
    }

    printf("%5.1f%%|", progress * 100);
    for (int i = 0; i < bar_width; i++) {
        if (i < filled) printf("█");
        else printf("░");
    }
    printf("| %s/%s | 🚀 %s | ⏱️ %.2fh | ETA: %s",
           bytes_str, total_str, speed_str, elapsed_hours, eta_str);

    if (sleeping) {
        printf(" (passive mode)");
    } else if (paused) {
        printf(" (paused)");
    }

    printf("    ");  // Clear any trailing chars
    fflush(stdout);
}

// Signal handler for clean exit
static void signal_handler(int sig) {
    (void)sig;
    atomic_store(&stop_flag, 1);
}

// Restore terminal settings
static void restore_terminal(void) {
    if (termios_saved) {
        tcsetattr(STDIN_FILENO, TCSADRAIN, &old_termios);
    }
}

// Setup terminal for raw input
static int setup_terminal(void) {
    struct termios new_termios;

    if (tcgetattr(STDIN_FILENO, &old_termios) < 0) {
        return -1;
    }
    termios_saved = 1;
    atexit(restore_terminal);

    new_termios = old_termios;
    new_termios.c_lflag &= ~(ICANON | ECHO);
    new_termios.c_cc[VMIN] = 0;
    new_termios.c_cc[VTIME] = 0;

    if (tcsetattr(STDIN_FILENO, TCSANOW, &new_termios) < 0) {
        return -1;
    }
    return 0;
}

int main(int argc, char *argv[]) {
    pthread_t threads[NUM_THREADS];
    pthread_t kb_thread;
    pthread_t passive_thread;
    int thread_count = 0;
    time_t start_time, current_time;
    unsigned long long last_bytes = 0;
    double pause_time = 0;
    time_t pause_start = 0;
    int passive_mode = 0;  // Default: active mode (normal)

    // Parse command-line arguments
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--passive") == 0) {
            passive_mode = 1;
        } else if (strcmp(argv[i], "--help") == 0 || strcmp(argv[i], "-h") == 0) {
            printf("Usage: %s [options]\n", argv[0]);
            printf("  --passive    Enable passive mode (default: active mode)\n");
            printf("  --help       Show this help message\n");
            return 0;
        }
    }

    // Initialize curl
    curl_global_init(CURL_GLOBAL_ALL);

    // Setup signal handlers
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);

    // Setup terminal for raw input
    if (setup_terminal() < 0) {
        printf("ℹ️  Keyboard controls not available in this terminal\n");
    }

    // Print header
    printf("============================================================\n");
    printf("🚀 HIGH-SPEED DATA DOWNLOADER (C Version)\n");
    printf("🎯 Target: %d GB\n", TARGET_GB);
    printf("📡 Using %lu download sources with %d threads each\n",
           (unsigned long)NUM_URLS, THREADS_PER_URL);
    printf("============================================================\n");
    printf("⚠️  Data is downloaded to RAM and discarded (no disk usage)\n");
    printf("============================================================\n");
    if (passive_mode) {
        printf("💤 PASSIVE MODE: Download %ds, Sleep %ds\n", DOWNLOAD_DURATION, SLEEP_DURATION);
    } else {
        printf("📥 ACTIVE MODE: Continuous downloading\n");
    }
    printf("============================================================\n");
    printf("🎮 Controls: Press 'p' to PAUSE/PLAY, 'q' to QUIT\n");
    printf("============================================================\n\n");

    start_time = time(NULL);

    // Start keyboard listener thread
    if (pthread_create(&kb_thread, NULL, keyboard_listener, NULL) != 0) {
        fprintf(stderr, "Failed to create keyboard listener thread\n");
    }

    if (passive_mode) {
        // Start passive mode manager thread
        if (pthread_create(&passive_thread, NULL, passive_mode_manager, NULL) != 0) {
            fprintf(stderr, "Failed to create passive mode manager thread\n");
        }
    }

    // Start download threads
    for (size_t i = 0; i < NUM_URLS; i++) {
        for (int j = 0; j < THREADS_PER_URL; j++) {
            thread_arg_t *arg = malloc(sizeof(thread_arg_t));
            if (!arg) continue;

            arg->url = TEST_FILES[i];
            arg->thread_id = thread_count;

            if (pthread_create(&threads[thread_count], NULL, download_thread, arg) == 0) {
                thread_count++;
            } else {
                free(arg);
            }
        }
    }

    printf("Started %d download threads\n\n", thread_count);

    // Progress monitoring loop
    double avg_speed_smooth = 0;  // Smoothed average speed for ETA
    while (!atomic_load(&stop_flag)) {
        unsigned long long current_bytes = atomic_load(&total_bytes);
        current_time = time(NULL);

        // Check if target reached
        if (current_bytes >= TARGET_BYTES) {
            atomic_store(&stop_flag, 1);
            break;
        }

        // Handle pause time tracking
        if (atomic_load(&pause_flag)) {
            if (pause_start == 0) {
                pause_start = current_time;
            }
        } else {
            if (pause_start != 0) {
                pause_time += difftime(current_time, pause_start);
                pause_start = 0;
            }
        }

        // Calculate elapsed time (excluding pause)
        double elapsed = difftime(current_time, start_time);
        if (pause_start != 0) {
            elapsed -= difftime(current_time, pause_start);
        }
        elapsed -= pause_time;
        double elapsed_hours = elapsed / 3600.0;

        // Calculate speed
        double speed = 0;
        if (elapsed > 0) {
            speed = (double)(current_bytes - last_bytes);  // bytes in last second
        }

        // Calculate smoothed average speed for better ETA
        if (avg_speed_smooth == 0 && speed > 0) {
            avg_speed_smooth = speed;
        } else if (speed > 0) {
            avg_speed_smooth = avg_speed_smooth * 0.9 + speed * 0.1;  // Exponential smoothing
        }

        // Calculate ETA
        double eta_seconds = -1;
        if (avg_speed_smooth > 0) {
            unsigned long long remaining_bytes = TARGET_BYTES - current_bytes;
            eta_seconds = (double)remaining_bytes / avg_speed_smooth;
        }

        // Print progress
        pthread_mutex_lock(&print_lock);
        print_progress(current_bytes, TARGET_BYTES, speed, elapsed_hours, eta_seconds,
                      atomic_load(&pause_flag), atomic_load(&passive_sleep_flag));
        pthread_mutex_unlock(&print_lock);

        last_bytes = current_bytes;

        sleep(1);
    }

    printf("\n\nWaiting for threads to finish...\n");

    // Wait for all threads
    for (int i = 0; i < thread_count; i++) {
        pthread_join(threads[i], NULL);
    }
    pthread_join(kb_thread, NULL);
    if (passive_mode) {
        pthread_join(passive_thread, NULL);
    }

    // Calculate final statistics
    unsigned long long final_bytes = atomic_load(&total_bytes);
    double elapsed = difftime(time(NULL), start_time) - pause_time;
    double hours = elapsed / 3600.0;
    double avg_speed = (elapsed > 0) ? (double)final_bytes / elapsed : 0;

    // Print summary
    char bytes_str[32], speed_str[32];
    format_bytes(final_bytes, bytes_str, sizeof(bytes_str));
    format_speed(avg_speed, speed_str, sizeof(speed_str));

    printf("\n============================================================\n");
    if (final_bytes >= TARGET_BYTES) {
        printf("🎉 TARGET REACHED: %d GB Downloaded!\n", TARGET_GB);
    } else {
        printf("📊 Downloaded: %s\n", bytes_str);
    }
    printf("============================================================\n");
    printf("📥 Total Downloaded: %s\n", bytes_str);
    printf("⏱️  Time Taken: %.2f hours (excluding pauses)\n", hours);
    printf("⏸️  Total Pause Time: %.1f minutes\n", pause_time / 60.0);
    printf("📈 Average Speed: %s\n", speed_str);
    printf("============================================================\n");

    curl_global_cleanup();
    return 0;
}
