import React from 'react';
import { render } from 'ink';
import { PlayerUI } from './ui/Player.js';
import { playerState } from './state.js';
import { db } from './db.js';
import { podcastPlayer } from './player.js';

// Demo: Create sample episodes
function setupDemoData() {
  const sampleEpisodes = [
    {
      id: 'ep-001',
      title: 'Episode 1: Getting Started with Podcasts',
      feed_url: 'https://example.com/feed',
      episode_url: 'https://example.com/episode1.mp3',
      duration: 2700, // 45 minutes
      current_time: 0,
      playback_speed: 1.0,
      is_played: false,
      queue_position: 1,
      published_date: new Date().toISOString(),
    },
    {
      id: 'ep-002',
      title: 'Episode 2: Advanced Topics',
      feed_url: 'https://example.com/feed',
      episode_url: 'https://example.com/episode2.mp3',
      duration: 3600, // 60 minutes
      current_time: 0,
      playback_speed: 1.0,
      is_played: false,
      queue_position: 2,
      published_date: new Date().toISOString(),
    },
    {
      id: 'ep-003',
      title: 'Episode 3: Q&A Session',
      feed_url: 'https://example.com/feed',
      episode_url: 'https://example.com/episode3.mp3',
      duration: 2400, // 40 minutes
      current_time: 0,
      playback_speed: 1.0,
      is_played: false,
      queue_position: 3,
      published_date: new Date().toISOString(),
    },
  ];

  // Insert episodes into database
  sampleEpisodes.forEach(episode => {
    db.insertEpisode(episode);
  });

  // Set current episode
  playerState.setCurrentEpisode('ep-001');
  playerState.refreshQueue();
}

async function main() {
  try {
    // Setup demo data
    setupDemoData();

    console.log('🎙️  Podcast Player with Progress, Speed Control & Queue ETA\n');

    // Render the UI
    const { unmount } = render(React.createElement(PlayerUI));

    // Simulate playback state updates for demo
    let demoTime = 0;
    const demoInterval = setInterval(() => {
      demoTime += 1;
      playerState.updateCurrentTime(demoTime);

      // Demo speed changes after 5 seconds
      if (demoTime === 5) {
        console.log('\n📊 Changing speed to 1.5x...');
        playerState.setSpeed(1.5);
      }

      // Demo speed changes after 10 seconds
      if (demoTime === 10) {
        console.log('\n📊 Changing speed to 2.0x...');
        playerState.setSpeed(2.0);
      }

      // Stop demo after 30 seconds
      if (demoTime >= 30) {
        clearInterval(demoInterval);
        setTimeout(() => {
          unmount();
          console.log('\n✅ Demo complete! Exiting...');
          process.exit(0);
        }, 1000);
      }
    }, 1000);

    // Handle keyboard input (for future control implementation)
    if (process.stdin.isTTY) {
      process.stdin.setRawMode(true);
      process.stdin.on('data', key => {
        const char = key.toString();

        switch (char) {
          case '1':
            playerState.setSpeed(1.0);
            console.log('\n🎚️  Speed: 1.0x');
            break;
          case '2':
            playerState.setSpeed(1.5);
            console.log('\n🎚️  Speed: 1.5x');
            break;
          case '3':
            playerState.setSpeed(2.0);
            console.log('\n🎚️  Speed: 2.0x');
            break;
          case 'q':
            clearInterval(demoInterval);
            unmount();
            console.log('\n👋 Goodbye!');
            process.exit(0);
            break;
        }
      });
    }
  } catch (error) {
    console.error('Error:', error);
    process.exit(1);
  }
}

main();
