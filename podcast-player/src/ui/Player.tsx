import React, { useEffect, useState } from 'react';
import { Box, Text } from 'ink';
import { ProgressBar } from './ProgressBar.js';
import { SpeedControl } from './SpeedControl.js';
import { QueueETA } from './QueueETA.js';
import { playerState, PlayerStateSnapshot } from '../state.js';

export const PlayerUI: React.FC = () => {
  const [state, setState] = useState<PlayerStateSnapshot>(playerState.getSnapshot());

  useEffect(() => {
    const unsubscribe = playerState.subscribe(newState => {
      setState(newState);
    });

    return () => {
      unsubscribe();
    };
  }, []);

  if (!state.currentEpisode) {
    return (
      <Box flexDirection="column">
        <Text>No episode selected</Text>
      </Box>
    );
  }

  const currentEpisode = state.currentEpisode;

  return (
    <Box flexDirection="column" gap={1} paddingBottom={1}>
      <Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1}>
        <Text bold color="cyan">
          {currentEpisode.title}
        </Text>
      </Box>

      <ProgressBar current={state.currentTime} total={currentEpisode.duration} width={40} />

      <SpeedControl />

      <QueueETA updateInterval={500} />

      <Box flexDirection="column" marginTop={1}>
        <Text dimColor>Controls: p (play) | s (pause) | q (quit)</Text>
        <Text dimColor>Speed: 1 (1.0x) | 2 (1.5x) | 3 (2.0x)</Text>
      </Box>
    </Box>
  );
};
