import initSqlJs, { Database as SqlJsDatabase } from 'sql.js';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const dbPath = path.join(__dirname, '..', 'podcast.db');

export interface Episode {
  id: string;
  title: string;
  feed_url: string;
  episode_url: string;
  duration: number; // seconds
  current_time: number; // seconds
  playback_speed: number; // 1.0, 1.5, 2.0, etc
  is_played: boolean;
  queue_position: number;
  published_date: string;
  created_at: string;
  updated_at: string;
}

export interface AppState {
  id: number;
  current_episode_id: string | null;
  current_speed: number;
  last_updated: string;
}

class PodcastDatabase {
  private db!: SqlJsDatabase;
  private initialized = false;

  async initialize(): Promise<void> {
    if (this.initialized) return;

    try {
      const SQL = await initSqlJs();

      // Load existing database if it exists
      if (fs.existsSync(dbPath)) {
        const buffer = fs.readFileSync(dbPath);
        this.db = new SQL.Database(buffer);
      } else {
        this.db = new SQL.Database();
      }

      this.initializeSchema();
      this.save();
      this.initialized = true;
    } catch (error) {
      console.error('Failed to initialize database:', error);
      throw error;
    }
  }

  private initializeSchema(): void {
    // Create episodes table
    this.db.run(`
      CREATE TABLE IF NOT EXISTS episodes (
        id TEXT PRIMARY KEY,
        title TEXT NOT NULL,
        feed_url TEXT NOT NULL,
        episode_url TEXT NOT NULL,
        duration INTEGER DEFAULT 0,
        current_time INTEGER DEFAULT 0,
        playback_speed REAL DEFAULT 1.0,
        is_played BOOLEAN DEFAULT 0,
        queue_position INTEGER,
        published_date TEXT,
        created_at TEXT DEFAULT CURRENT_TIMESTAMP,
        updated_at TEXT DEFAULT CURRENT_TIMESTAMP
      );
    `);

    // Create app_state table
    this.db.run(`
      CREATE TABLE IF NOT EXISTS app_state (
        id INTEGER PRIMARY KEY CHECK (id = 1),
        current_episode_id TEXT,
        current_speed REAL DEFAULT 1.0,
        last_updated TEXT DEFAULT CURRENT_TIMESTAMP
      );
    `);

    // Ensure one row in app_state
    try {
      const result = this.db.exec(
        'SELECT COUNT(*) as count FROM app_state'
      );
      const count = result[0]?.values[0]?.[0] as number || 0;

      if (count === 0) {
        this.db.run(
          'INSERT INTO app_state (id, current_episode_id, current_speed) VALUES (1, NULL, 1.0)'
        );
      }
    } catch (e) {
      // Table might not exist yet, try inserting anyway
      this.db.run(
        'INSERT OR IGNORE INTO app_state (id, current_episode_id, current_speed) VALUES (1, NULL, 1.0)'
      );
    }
  }

  private save(): void {
    try {
      const data = this.db.export();
      const buffer = Buffer.from(data);
      fs.writeFileSync(dbPath, buffer);
    } catch (error) {
      console.error('Failed to save database:', error);
    }
  }

  // Episode CRUD
  getEpisodeById(id: string): Episode | undefined {
    try {
      const stmt = this.db.prepare('SELECT * FROM episodes WHERE id = ?');
      stmt.bind([id]);

      if (stmt.step()) {
        const row = stmt.getAsObject() as Episode;
        stmt.free();
        return row;
      }

      stmt.free();
      return undefined;
    } catch (error) {
      console.error('Error fetching episode:', error);
      return undefined;
    }
  }

  getAllEpisodes(): Episode[] {
    try {
      const stmt = this.db.prepare(
        'SELECT * FROM episodes ORDER BY published_date DESC'
      );
      const episodes: Episode[] = [];

      while (stmt.step()) {
        episodes.push(stmt.getAsObject() as Episode);
      }

      stmt.free();
      return episodes;
    } catch (error) {
      console.error('Error fetching all episodes:', error);
      return [];
    }
  }

  getQueueEpisodes(): Episode[] {
    try {
      const stmt = this.db.prepare(
        'SELECT * FROM episodes WHERE is_played = 0 ORDER BY queue_position ASC, published_date DESC'
      );
      const episodes: Episode[] = [];

      while (stmt.step()) {
        episodes.push(stmt.getAsObject() as Episode);
      }

      stmt.free();
      return episodes;
    } catch (error) {
      console.error('Error fetching queue episodes:', error);
      return [];
    }
  }

  insertEpisode(episode: Omit<Episode, 'created_at' | 'updated_at'>): void {
    try {
      const stmt = this.db.prepare(`
        INSERT OR REPLACE INTO episodes 
        (id, title, feed_url, episode_url, duration, current_time, playback_speed, is_played, queue_position, published_date)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `);

      stmt.bind([
        episode.id,
        episode.title,
        episode.feed_url,
        episode.episode_url,
        episode.duration,
        episode.current_time,
        episode.playback_speed,
        episode.is_played ? 1 : 0,
        episode.queue_position,
        episode.published_date,
      ]);

      stmt.step();
      stmt.free();
      this.save();
    } catch (error) {
      console.error('Error inserting episode:', error);
    }
  }

  updateEpisodeTime(episodeId: string, currentTime: number): void {
    try {
      const stmt = this.db.prepare(`
        UPDATE episodes 
        SET current_time = ?, updated_at = CURRENT_TIMESTAMP 
        WHERE id = ?
      `);

      stmt.bind([currentTime, episodeId]);
      stmt.step();
      stmt.free();
      this.save();
    } catch (error) {
      console.error('Error updating episode time:', error);
    }
  }

  updatePlaybackSpeed(episodeId: string, speed: number): void {
    try {
      const stmt = this.db.prepare(`
        UPDATE episodes 
        SET playback_speed = ?, updated_at = CURRENT_TIMESTAMP 
        WHERE id = ?
      `);

      stmt.bind([speed, episodeId]);
      stmt.step();
      stmt.free();
      this.save();
    } catch (error) {
      console.error('Error updating playback speed:', error);
    }
  }

  markEpisodeAsPlayed(episodeId: string): void {
    try {
      const stmt = this.db.prepare(`
        UPDATE episodes 
        SET is_played = 1, updated_at = CURRENT_TIMESTAMP 
        WHERE id = ?
      `);

      stmt.bind([episodeId]);
      stmt.step();
      stmt.free();
      this.save();
    } catch (error) {
      console.error('Error marking episode as played:', error);
    }
  }

  updateQueuePosition(episodeId: string, position: number): void {
    try {
      const stmt = this.db.prepare(`
        UPDATE episodes 
        SET queue_position = ?, updated_at = CURRENT_TIMESTAMP 
        WHERE id = ?
      `);

      stmt.bind([position, episodeId]);
      stmt.step();
      stmt.free();
      this.save();
    } catch (error) {
      console.error('Error updating queue position:', error);
    }
  }

  // App State CRUD
  getAppState(): AppState {
    try {
      const stmt = this.db.prepare('SELECT * FROM app_state WHERE id = 1');

      if (stmt.step()) {
        const state = stmt.getAsObject() as AppState;
        stmt.free();
        return state;
      }

      stmt.free();
      return {
        id: 1,
        current_episode_id: null,
        current_speed: 1.0,
        last_updated: new Date().toISOString(),
      };
    } catch (error) {
      console.error('Error fetching app state:', error);
      return {
        id: 1,
        current_episode_id: null,
        current_speed: 1.0,
        last_updated: new Date().toISOString(),
      };
    }
  }

  setCurrentEpisode(episodeId: string | null): void {
    try {
      const stmt = this.db.prepare(`
        UPDATE app_state 
        SET current_episode_id = ?, last_updated = CURRENT_TIMESTAMP 
        WHERE id = 1
      `);

      stmt.bind([episodeId]);
      stmt.step();
      stmt.free();
      this.save();
    } catch (error) {
      console.error('Error setting current episode:', error);
    }
  }

  setCurrentSpeed(speed: number): void {
    try {
      const stmt = this.db.prepare(`
        UPDATE app_state 
        SET current_speed = ?, last_updated = CURRENT_TIMESTAMP 
        WHERE id = 1
      `);

      stmt.bind([speed]);
      stmt.step();
      stmt.free();
      this.save();
    } catch (error) {
      console.error('Error setting current speed:', error);
    }
  }

  close(): void {
    try {
      this.save();
      this.db.close();
    } catch (error) {
      console.error('Error closing database:', error);
    }
  }
}

// Create and initialize the database instance
const dbInstance = new PodcastDatabase();
await dbInstance.initialize();

export const db = dbInstance;
