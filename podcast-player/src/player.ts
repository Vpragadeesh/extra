import { spawn, ChildProcess } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';
import { playerState } from './state.js';
import { db } from './db.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const SUPPORTED_SPEEDS = [0.75, 1.0, 1.25, 1.5, 2.0];

export class PodcastPlayer {
  private currentProcess: ChildProcess | null = null;
  private currentEpisodeId: string | null = null;
  private currentSpeed: number = 1.0;
  private startTime: number = 0;
  private pausedTime: number = 0;
  private isPaused: boolean = false;
  private pollInterval: NodeJS.Timeout | null = null;

  constructor() {
    playerState.on('speedChanged', (speed: number) => {
      if (this.currentProcess) {
        this.handleSpeedChange(speed);
      }
    });
  }

  async play(episodeId: string, episodeUrl: string): Promise<void> {
    try {
      // Stop current playback
      if (this.currentProcess) {
        this.stop();
      }

      const episode = db.getEpisodeById(episodeId);
      if (!episode) {
        throw new Error(`Episode ${episodeId} not found`);
      }

      this.currentEpisodeId = episodeId;
      this.currentSpeed = episode.playback_speed || 1.0;

      playerState.setCurrentEpisode(episodeId);
      playerState.setIsPlaying(true);

      // Start playback with ffmpeg if speed is not 1.0
      if (this.currentSpeed !== 1.0) {
        await this.playWithSpeed(episodeUrl, this.currentSpeed);
      } else {
        await this.playDirect(episodeUrl);
      }

      this.startPolling();
    } catch (error) {
      console.error('Playback error:', error);
      playerState.setIsPlaying(false);
    }
  }

  private async playDirect(episodeUrl: string): Promise<void> {
    // Use ffplay (comes with ffmpeg) for direct playback
    this.currentProcess = spawn('ffplay', [
      '-nodisp', // No display window
      '-autoexit', // Exit when playback ends
      '-loglevel',
      'error', // Suppress output
      episodeUrl,
    ]);

    this.startTime = Date.now();

    this.currentProcess.on('close', () => {
      this.currentProcess = null;
      if (this.pollInterval) clearInterval(this.pollInterval);
      playerState.setIsPlaying(false);
    });

    this.currentProcess.on('error', error => {
      console.error('Player error:', error);
      playerState.setIsPlaying(false);
    });
  }

  private async playWithSpeed(episodeUrl: string, speed: number): Promise<void> {
    // Use ffplay with atempo filter for speed control
    const tempoFilter = `atempo=${Math.min(Math.max(speed, 0.5), 2.0)}`; // Constrain speed

    this.currentProcess = spawn('ffplay', [
      '-nodisp',
      '-autoexit',
      '-loglevel',
      'error',
      '-af',
      tempoFilter,
      episodeUrl,
    ]);

    this.startTime = Date.now();

    this.currentProcess.on('close', () => {
      this.currentProcess = null;
      if (this.pollInterval) clearInterval(this.pollInterval);
      playerState.setIsPlaying(false);
    });

    this.currentProcess.on('error', error => {
      console.error('Player error:', error);
      playerState.setIsPlaying(false);
    });
  }

  private handleSpeedChange(newSpeed: number): void {
    // Speed changes mid-playback require stopping and restarting
    // Save current progress
    const currentTime = this.getCurrentTime();

    if (this.currentEpisodeId) {
      db.updatePlaybackSpeed(this.currentEpisodeId, newSpeed);
      this.currentSpeed = newSpeed;

      // Stop and restart with new speed
      const episode = db.getEpisodeById(this.currentEpisodeId);
      if (episode) {
        this.stop();
        // Note: In a real implementation, you'd resume from the saved position
        // For now, we just note that the speed changed
        console.log(`Speed changed to ${newSpeed}x. Restart playback to apply.`);
      }
    }
  }

  pause(): void {
    if (this.currentProcess && !this.isPaused) {
      // Send SIGSTOP to pause (Unix/Linux only)
      this.currentProcess.kill('SIGSTOP');
      this.pausedTime = this.getCurrentTime();
      this.isPaused = true;
      playerState.setIsPlaying(false);
    }
  }

  resume(): void {
    if (this.currentProcess && this.isPaused) {
      // Send SIGCONT to resume (Unix/Linux only)
      this.currentProcess.kill('SIGCONT');
      this.startTime = Date.now() - this.pausedTime * 1000;
      this.isPaused = false;
      playerState.setIsPlaying(true);
    }
  }

  stop(): void {
    if (this.currentProcess) {
      this.currentProcess.kill();
      this.currentProcess = null;
    }

    if (this.pollInterval) {
      clearInterval(this.pollInterval);
      this.pollInterval = null;
    }

    this.currentEpisodeId = null;
    playerState.setIsPlaying(false);
  }

  seek(seconds: number): void {
    if (this.currentEpisodeId) {
      playerState.updateCurrentTime(seconds);

      // Note: Seeking requires stopping and restarting from the new position
      // In a production implementation, you'd use mpv with socket control for seeking
      console.log(`Seek to ${seconds}s. Restart playback to apply.`);
    }
  }

  setSpeed(speed: number): void {
    if (!SUPPORTED_SPEEDS.includes(speed)) {
      console.error(
        `Unsupported speed: ${speed}. Supported: ${SUPPORTED_SPEEDS.join(', ')}`
      );
      return;
    }

    playerState.setSpeed(speed);
  }

  private startPolling(): void {
    if (this.pollInterval) clearInterval(this.pollInterval);

    this.pollInterval = setInterval(() => {
      const currentTime = this.getCurrentTime();
      playerState.updateCurrentTime(currentTime);
    }, 500); // Update every 500ms
  }

  private getCurrentTime(): number {
    if (!this.currentProcess || this.isPaused) {
      return this.pausedTime;
    }

    if (this.startTime === 0) return 0;

    // Calculate elapsed time based on wall clock
    const elapsedMs = Date.now() - this.startTime;
    const elapsedSeconds = elapsedMs / 1000;

    // Account for speed - if playing at 1.5x, wall time progresses faster
    // But ffplay with atempo filter already handles this in the output
    // So we just return elapsed seconds normally
    return elapsedSeconds;
  }

  getDuration(episodeId: string): number {
    const episode = db.getEpisodeById(episodeId);
    return episode?.duration || 0;
  }

  isPlaying(): boolean {
    return this.currentProcess !== null && !this.isPaused;
  }

  getCurrentEpisodeId(): string | null {
    return this.currentEpisodeId;
  }

  getCurrentSpeed(): number {
    return this.currentSpeed;
  }
}

export const podcastPlayer = new PodcastPlayer();
