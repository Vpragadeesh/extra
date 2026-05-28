import { db, Episode, AppState } from './db.js';
import { EventEmitter } from 'events';

export interface PlayerStateSnapshot {
  currentEpisodeId: string | null;
  currentEpisode: Episode | null;
  currentSpeed: number;
  currentTime: number;
  queue: Episode[];
  isPlaying: boolean;
}

type StateListener = (state: PlayerStateSnapshot) => void;

class PlayerState extends EventEmitter {
  private currentEpisodeId: string | null = null;
  private currentSpeed: number = 1.0;
  private currentTime: number = 0;
  private isPlaying: boolean = false;
  private queue: Episode[] = [];
  private listeners: Set<StateListener> = new Set();

  constructor() {
    super();
    this.loadFromDb();
  }

  private loadFromDb(): void {
    const appState = db.getAppState();
    this.currentEpisodeId = appState.current_episode_id;
    this.currentSpeed = appState.current_speed;
    this.refreshQueue();
  }

  // Public API
  setCurrentEpisode(episodeId: string | null): void {
    this.currentEpisodeId = episodeId;
    db.setCurrentEpisode(episodeId);
    this.currentTime = 0;

    if (episodeId) {
      const episode = db.getEpisodeById(episodeId);
      if (episode) {
        this.currentTime = episode.current_time;
        this.currentSpeed = episode.playback_speed;
      }
    }

    this.notifyListeners();
  }

  setSpeed(speed: number): void {
    this.currentSpeed = speed;
    db.setCurrentSpeed(speed);

    if (this.currentEpisodeId) {
      db.updatePlaybackSpeed(this.currentEpisodeId, speed);
    }

    this.notifyListeners();
    this.emit('speedChanged', speed);
  }

  updateCurrentTime(time: number): void {
    this.currentTime = time;

    if (this.currentEpisodeId) {
      db.updateEpisodeTime(this.currentEpisodeId, Math.floor(time));
    }

    this.notifyListeners();
  }

  setIsPlaying(playing: boolean): void {
    this.isPlaying = playing;
    this.notifyListeners();
  }

  refreshQueue(): void {
    this.queue = db.getQueueEpisodes();
    this.notifyListeners();
  }

  markCurrentAsPlayed(): void {
    if (this.currentEpisodeId) {
      db.markEpisodeAsPlayed(this.currentEpisodeId);
      this.refreshQueue();
    }
  }

  getSnapshot(): PlayerStateSnapshot {
    const currentEpisode = this.currentEpisodeId ? db.getEpisodeById(this.currentEpisodeId) : null;

    return {
      currentEpisodeId: this.currentEpisodeId,
      currentEpisode: currentEpisode || null,
      currentSpeed: this.currentSpeed,
      currentTime: this.currentTime,
      queue: this.queue,
      isPlaying: this.isPlaying,
    };
  }

  // Listener management
  subscribe(listener: StateListener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  private notifyListeners(): void {
    const snapshot = this.getSnapshot();
    this.listeners.forEach(listener => listener(snapshot));
  }

  // Utility calculations
  getCurrentEpisodeETA(): number {
    const episode = this.currentEpisodeId ? db.getEpisodeById(this.currentEpisodeId) : null;
    if (!episode) return 0;

    const remaining = Math.max(0, episode.duration - this.currentTime);
    return remaining / this.currentSpeed;
  }

  getQueueETA(): number {
    let total = 0;

    for (const episode of this.queue) {
      let remaining = Math.max(0, episode.duration - episode.current_time);

      // If this is the current episode being played, use currentTime
      if (episode.id === this.currentEpisodeId) {
        remaining = Math.max(0, episode.duration - this.currentTime);
      }

      total += remaining / this.currentSpeed;
    }

    return total;
  }

  close(): void {
    this.listeners.clear();
    this.removeAllListeners();
  }
}

export const playerState = new PlayerState();
