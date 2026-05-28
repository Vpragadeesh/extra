import React, { useEffect, useState } from 'react';
import { Box, Text } from 'ink';
import { playerState, PlayerStateSnapshot } from '../state.js';

const SPEEDS = [0.75, 1.0, 1.25, 1.5, 2.0];

interface SpeedControlProps {
  onSpeedChange?: (speed: number) => void;
}

export const SpeedControl: React.FC<SpeedControlProps> = ({ onSpeedChange }) => {
  const [state, setState] = useState<PlayerStateSnapshot>(playerState.getSnapshot());

  useEffect(() => {
    const unsubscribe = playerState.subscribe(newState => {
      setState(newState);
    });

    const speedChangeHandler = (speed: number) => {
      onSpeedChange?.(speed);
    };

    playerState.on('speedChanged', speedChangeHandler);

    return () => {
      unsubscribe();
      playerState.removeListener('speedChanged', speedChangeHandler);
    };
  }, [onSpeedChange]);

  return (
    <Box flexDirection="row" gap={1}>
      <Text>Speed: </Text>
      {SPEEDS.map(speed => (
        <Text
          key={speed}
          backgroundColor={state.currentSpeed === speed ? 'blue' : 'gray'}
          color="white"
        >
          {speed.toFixed(2)}x
        </Text>
      ))}
    </Box>
  );
};
