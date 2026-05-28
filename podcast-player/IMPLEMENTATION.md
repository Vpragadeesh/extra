# Podcast Player: Progress Bar + Speed Control + Queue ETA

## Overview

This is a complete implementation of a CLI podcast player with:
- **Progress Bar**: Visual display of current playback position (e.g., `[████░░░░░░] 15:30 / 45:00 (34%)`)
- **Playback Speed Control**: Support for 5 playable speeds (0.75x, 1.0x, 1.25x, 1.5x, 2.0x)
- **Queue ETA**: Real-time estimated time to complete all queued episodes, accounting for speed factor

## Project Structure

```
src/
├── index.ts              # Entry point with demo
├── db.ts                 # SQLite database layer (CRUD operations)
├── state.ts              # Global state management (PlayerState singleton)
├── player.ts             # Audio playback engine with speed control
└── ui/
    ├── ProgressBar.tsx   # Progress bar component
    ├── SpeedControl.tsx  # Speed selector component
    ├── QueueETA.tsx      # Queue ETA display component
    └── Player.tsx        # Main integrated player UI
```

## Architecture

### 1. Database Layer (`db.ts`)

Manages all data persistence using SQLite.

**Schema:**
```sql
-- Episodes table
episodes:
  - id (TEXT PRIMARY KEY)
  - title (TEXT)
  - feed_url (TEXT)
  - episode_url (TEXT)
  - duration (INTEGER seconds)
  - current_time (INTEGER seconds)
  - playback_speed (REAL default 1.0)
  - is_played (BOOLEAN)
  - queue_position (INTEGER)
  - published_date (TEXT)
  - created_at (TEXT)
  - updated_at (TEXT)

-- App state table
app_state:
  - id (INTEGER PRIMARY KEY)
  - current_episode_id (TEXT nullable)
  - current_speed (REAL default 1.0)
  - last_updated (TEXT)
```

**Key Methods:**
- `getEpisodeById(id)` - Fetch single episode
- `getQueueEpisodes()` - Get all unplayed episodes
- `updateEpisodeTime(episodeId, time)` - Track playback position
- `updatePlaybackSpeed(episodeId, speed)` - Persist speed preference
- `getAppState()` / `setCurrentSpeed()` - Global state management

### 2. State Management (`state.ts`)

`PlayerState` is a singleton EventEmitter that manages global player state.

**State Structure:**
```typescript
{
  currentEpisodeId: string | null;
  currentEpisode: Episode | null;
  currentSpeed: number;
  currentTime: number;
  queue: Episode[];
  isPlaying: boolean;
}
```

**Key Features:**
- Subscriber pattern: `playerState.subscribe(listener)` returns unsubscribe function
- Event emissions: `speedChanged` event when speed changes
- ETA calculations:
  - `getCurrentEpisodeETA()` - Remaining time for current episode
  - `getQueueETA()` - Total remaining time for all queued episodes (accounting for speed)

**Example:**
```typescript
const unsubscribe = playerState.subscribe((state) => {
  console.log(`Playing: ${state.currentEpisode?.title}`);
  console.log(`Queue ETA: ${playerState.getQueueETA()}s`);
});

playerState.setSpeed(1.5); // Triggers speedChanged event
```

### 3. Player Engine (`player.ts`)

`PodcastPlayer` handles audio playback with speed control.

**Playback Implementation:**
- Uses `ffplay` (from FFmpeg suite) for audio playback
- Speed control via `atempo` audio filter (e.g., `-af "atempo=1.5"`)
- Polling mechanism updates current time every 500ms
- Support for pause/resume (using SIGSTOP/SIGCONT)

**Speed Support:**
| Speed | Filter | Notes |
|-------|--------|-------|
| 0.75x | `atempo=0.75` | Slower playback |
| 1.0x  | None | Normal speed |
| 1.25x | `atempo=1.25` | Slightly faster |
| 1.5x  | `atempo=1.5` | Faster |
| 2.0x  | `atempo=2.0` | Double speed |

**Methods:**
- `play(episodeId, url)` - Start playback
- `pause()` / `resume()` - Pause/resume
- `stop()` - Stop playback
- `setSpeed(speed)` - Change speed (requires restart)
- `seek(seconds)` - Seek to position (requires restart)

### 4. UI Components

All components are built with React + Ink for terminal rendering.

#### ProgressBar (`ui/ProgressBar.tsx`)
```
[████████░░░░░░░░░░░░░░░░░░] 15:30 / 45:00 (34%)
```
- Fixed 40-character width bar
- Shows current/total time in MM:SS format
- Updates every 500ms

#### SpeedControl (`ui/SpeedControl.tsx`)
```
Speed: 0.75x 1.0x [1.5x] 2.0x
```
- Displays 5 speed options
- Highlights current speed (blue background)
- Non-interactive display (parent handles input)

#### QueueETA (`ui/QueueETA.tsx`)
```
Queue ETA: 12h 45m (at 1.5x)
Episodes in queue: 3
```
- Recalculates every 500ms
- Accounts for speed factor in calculation
- Shows queue count

#### PlayerUI (`ui/Player.tsx`)
Integrates all components:
```
╭─ Episode 1: Getting Started with Podcasts ─╮
│ [████░░░░░░] 15:30 / 45:00 (34%)           │
│ Speed: 0.75x 1.0x [1.5x] 2.0x              │
│ Queue ETA: 45m 30s (at 1.5x)               │
│ Episodes in queue: 2                        │
╰─────────────────────────────────────────────╯
Controls: p (play) | s (pause) | q (quit)
Speed: 1 (1.0x) | 2 (1.5x) | 3 (2.0x)
```

## Key Calculations

### Progress Percentage
```typescript
percent = (currentTime / totalDuration) * 100
```

### Current Episode ETA
```typescript
remainingTime = (duration - currentTime) / speed
```

### Queue ETA (All Episodes)
```typescript
queueETA = sum([
  (episode.duration - episode.current_time) / speed 
  for episode in queue
])
```

**Example:**
- Episode 1: 45 min total, 15 min played → 30 min remaining
- Episode 2: 60 min total, 0 min played → 60 min remaining
- Episode 3: 40 min total, 0 min played → 40 min remaining
- At 1.5x speed:
  - Episode 1: 30 / 1.5 = 20 min
  - Episode 2: 60 / 1.5 = 40 min
  - Episode 3: 40 / 1.5 = 26.67 min
  - **Total: 86.67 min ≈ 1h 27m**

## Usage

### Installation
```bash
npm install
```

### Development
```bash
npm run dev
```

### Build
```bash
npm run build
```

### Run
```bash
npm start
```

## Demo Features

The `index.ts` entry point includes a demo that:

1. Creates 3 sample episodes in the database
2. Starts playback on Episode 1
3. Simulates time progression (1 second per 1 second of real time)
4. Changes speed to 1.5x after 5 seconds
5. Changes speed to 2.0x after 10 seconds
6. Exits after 30 seconds

**Interactive Controls (during demo):**
- `1` - Set speed to 1.0x
- `2` - Set speed to 1.5x
- `3` - Set speed to 2.0x
- `q` - Quit

## Implementation Details

### Speed Changes Mid-Playback
Speed changes require stopping and restarting the audio playback:

```typescript
// Speed change workflow:
1. User calls playerState.setSpeed(1.5)
2. PlayerState emits 'speedChanged' event
3. PodcastPlayer receives event
4. Current time is saved to DB
5. Player stops ffplay process
6. Player restarts ffplay with new atempo filter
7. UI updates automatically via subscriber pattern
```

### Time Tracking
Current playback position is calculated using wall-clock time:

```typescript
elapsedSeconds = (Date.now() - startTime) / 1000
```

Note: With `ffplay` + `atempo`, the audio output is already sped up, so wall-clock elapsed time directly reflects the audio being played.

### State Persistence
- Speed preference is saved to `episodes.playback_speed`
- Current position is saved to `episodes.current_time`
- Episode play state is tracked in `episodes.is_played`
- Global speed is persisted in `app_state.current_speed`

## Dependencies

- **ink**: Terminal UI rendering (React for CLI)
- **react**: Component framework
- **better-sqlite3**: SQLite database (synchronous for simplicity)
- **ffmpeg**: Audio playback and speed control
  - Requires `ffplay` in PATH for playback
  - Requires `ffmpeg` CLI for preprocessing if needed

### Install FFmpeg

**macOS:**
```bash
brew install ffmpeg
```

**Ubuntu/Debian:**
```bash
sudo apt-get install ffmpeg
```

**Windows:**
```bash
choco install ffmpeg
```

## Advanced Features (Future)

1. **Seek Control**: Use `mpv --input-ipc-server` for seeking support
2. **Real Playback**: Replace demo with actual podcast feed integration
3. **Bookmarks**: Save/restore playback positions by episode
4. **Smart Speed**: Adjust speed based on content type
5. **Sync**: Cloud sync of playback state across devices
6. **Transcripts**: Display episode transcripts with time-synced highlighting

## Testing

### Test Speed Calculations
```typescript
// ETA should account for speed
const ep1 = db.getEpisodeById('ep-001'); // 45 min duration
playerState.setCurrentEpisode('ep-001');
playerState.updateCurrentTime(15 * 60); // 15 min played
playerState.setSpeed(1.5);

const eta = playerState.getCurrentEpisodeETA();
// Expected: (45*60 - 15*60) / 1.5 = 1200 seconds = 20 minutes
```

### Test Speed Changes
```typescript
playerState.setSpeed(1.0); // Normal
playerState.setSpeed(1.5); // Faster
// UI should update automatically
// Play should restart with new atempo filter
```

### Test Queue ETA
```typescript
const queueETA = playerState.getQueueETA();
// Should include all unplayed episodes adjusted for speed
// Should recalculate as currentTime changes
```

## Error Handling

- Missing episodes: Returns graceful error message
- FFmpeg not installed: Falls back to documentation
- Invalid speed: Logs error and maintains current speed
- Database errors: Caught and logged to console

## File Reference Guide

| File | Purpose |
|------|---------|
| `src/db.ts` | Database initialization, schema, CRUD operations |
| `src/state.ts` | Global player state, ETA calculations, event emitting |
| `src/player.ts` | Audio playback, speed control, time polling |
| `src/ui/ProgressBar.tsx` | Progress visualization component |
| `src/ui/SpeedControl.tsx` | Speed option display component |
| `src/ui/QueueETA.tsx` | Queue time estimation component |
| `src/ui/Player.tsx` | Main integrated UI layout |
| `src/index.ts` | Entry point with demo |

## Checklist - Complete ✅

- [x] Update DB schema (playback_speed, current_time, queue_position)
- [x] Implement `PodcastPlayer` class with speed control
- [x] Build `ProgressBar` component (40-char bar, time display)
- [x] Build `SpeedControl` component (5 speed options)
- [x] Build `QueueETA` component (recalculate every 500ms)
- [x] Integrate all into main `PlayerUI`
- [x] Implement speed changes with event system
- [x] Implement ETA accuracy calculations
- [x] Handle speed change while playing
- [x] Document player implementation

## Next Steps

1. Install dependencies: `npm install`
2. Run demo: `npm run dev`
3. Integrate with actual podcast feeds (RSS parsing)
4. Add keyboard input handling for user controls
5. Implement playlist/queue management UI
6. Add episode search and filtering
