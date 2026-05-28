# Quick Start Guide

## Setup

```bash
# 1. Install dependencies
npm install

# 2. Ensure FFmpeg is installed
# macOS: brew install ffmpeg
# Ubuntu: sudo apt-get install ffmpeg
# Windows: choco install ffmpeg

# 3. Run the demo
npm run dev
```

## What You'll See

The demo will display a podcast player UI with:

```
🎙️  Podcast Player with Progress, Speed Control & Queue ETA

╭─ Episode 1: Getting Started with Podcasts ─╮
│ [████░░░░░░] 15:30 / 45:00 (34%)           │
│ Speed: 0.75x 1.0x [1.5x] 2.0x              │
│ Queue ETA: 45m 30s (at 1.5x)               │
│ Episodes in queue: 2                        │
╰─────────────────────────────────────────────╯
Controls: p (play) | s (pause) | q (quit)
Speed: 1 (1.0x) | 2 (1.5x) | 3 (2.0x)
```

## Live Demo Sequence

1. **0-5 seconds**: Normal playback at 1.0x speed
2. **5 seconds**: Speed changes to 1.5x (watch ETA decrease!)
3. **10 seconds**: Speed changes to 2.0x (ETA decreases more!)
4. **30 seconds**: Demo exits

## How to Use in Your App

### 1. Initialize the Player

```typescript
import { playerState } from './state.js';
import { db } from './db.js';

// Add an episode
db.insertEpisode({
  id: 'my-episode',
  title: 'My Episode',
  feed_url: 'https://podcast.com/feed',
  episode_url: 'https://podcast.com/episode.mp3',
  duration: 3600, // seconds
  current_time: 0,
  playback_speed: 1.0,
  is_played: false,
  queue_position: 1,
  published_date: new Date().toISOString(),
});

// Set as current and refresh queue
playerState.setCurrentEpisode('my-episode');
playerState.refreshQueue();
```

### 2. Listen to State Changes

```typescript
const unsubscribe = playerState.subscribe(state => {
  console.log(`Playing: ${state.currentEpisode?.title}`);
  console.log(`Progress: ${state.currentTime}s / ${state.currentEpisode?.duration}s`);
  console.log(`Queue ETA: ${playerState.getQueueETA()}s at ${state.currentSpeed}x`);
});
```

### 3. Control Playback

```typescript
import { podcastPlayer } from './player.js';

// Play episode
await podcastPlayer.play('my-episode', 'https://podcast.com/episode.mp3');

// Change speed
playerState.setSpeed(1.5);

// Pause/resume
podcastPlayer.pause();
podcastPlayer.resume();

// Stop
podcastPlayer.stop();

// Check state
console.log(podcastPlayer.isPlaying()); // boolean
console.log(podcastPlayer.getCurrentSpeed()); // number
```

### 4. Render UI

```typescript
import React from 'react';
import { render } from 'ink';
import { PlayerUI } from './ui/Player.js';

// In your main app
const { unmount } = render(React.createElement(PlayerUI));

// Later, when done
unmount();
```

## Key APIs

### PlayerState

```typescript
// Getters
playerState.getSnapshot() // Returns full state snapshot
playerState.getQueueETA() // Returns ETA in seconds
playerState.getCurrentEpisodeETA() // Current episode ETA

// Setters
playerState.setCurrentEpisode(id) // Set which episode to play
playerState.setSpeed(speed) // 0.75, 1.0, 1.25, 1.5, 2.0
playerState.updateCurrentTime(seconds) // Update playback position
playerState.setIsPlaying(bool)
playerState.refreshQueue() // Reload queue from DB
playerState.markCurrentAsPlayed()

// Event listening
playerState.subscribe(listener) // Returns unsubscribe function
playerState.on('speedChanged', (speed) => {})
```

### PodcastPlayer

```typescript
// Playback control
podcastPlayer.play(episodeId, url) // Start playback
podcastPlayer.pause()
podcastPlayer.resume()
podcastPlayer.stop()
podcastPlayer.seek(seconds)

// Info
podcastPlayer.isPlaying() // boolean
podcastPlayer.getCurrentEpisodeId() // string | null
podcastPlayer.getCurrentSpeed() // number
podcastPlayer.getDuration(episodeId) // number
```

### Database (db)

```typescript
// Episode CRUD
db.getEpisodeById(id)
db.getAllEpisodes()
db.getQueueEpisodes() // Unplayed episodes
db.insertEpisode(episode)
db.updateEpisodeTime(id, time)
db.updatePlaybackSpeed(id, speed)
db.markEpisodeAsPlayed(id)

// State
db.getAppState()
db.setCurrentEpisode(id)
db.setCurrentSpeed(speed)
```

## File Locations

- **Database**: `./podcast.db` (created on first run)
- **Source code**: `./src/`
- **Build output**: `./dist/`
- **Implementation docs**: `./IMPLEMENTATION.md`

## ETA Examples

Given these episodes:
- Episode 1: 45 min (15 min played) → 30 min remaining
- Episode 2: 60 min (0 min played) → 60 min remaining  
- Episode 3: 40 min (0 min played) → 40 min remaining

**Queue ETA at different speeds:**

| Speed | Calculation | ETA |
|-------|-------------|-----|
| 1.0x | 30 + 60 + 40 = 130 min | **2h 10m** |
| 1.5x | 20 + 40 + 26.67 = 86.67 min | **1h 27m** ⚡ |
| 2.0x | 15 + 30 + 20 = 65 min | **1h 5m** ⚡⚡ |

## Troubleshooting

**FFmpeg not found:**
```bash
# Check if ffplay is available
which ffplay

# If not, install FFmpeg for your OS
# See IMPLEMENTATION.md for platform-specific instructions
```

**Database locked error:**
```bash
# Delete the database to start fresh
rm podcast.db podcast.db-shm podcast.db-wal

# Try again
npm run dev
```

**React/Ink rendering issues:**
```bash
# Rebuild and try again
npm run build
npm start
```

## What's Next?

1. ✅ Implement core podcast player features
2. 🔜 Add RSS feed parsing to fetch real episodes
3. 🔜 Build interactive CLI controls
4. 🔜 Add episode search and filtering
5. 🔜 Cloud sync for playback state
6. 🔜 Episode bookmarks and notes

Enjoy your podcast player! 🎧
