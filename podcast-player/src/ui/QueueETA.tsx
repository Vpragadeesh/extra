import React, { useEffect, useState } from 'react';
import { Box, Text } from 'ink';
import { playerState, PlayerStateSnapshot } from '../state.js';

interface QueueETAProps {
  updateInterval?: number; // milliseconds, default 500
}

const formatETA = (seconds: number): string => {
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);

  if (hours > 0) {
    return `${hours}h ${mins}m`;
  }

  return `${mins}m`;
};

export const QueueETA: React.FC<QueueETAProps> = ({ updateInterval = 500 }) => {
  const [etaSeconds, setEta] = useState(0);
  const [state, setState] = useState<PlayerStateSnapshot>(playerState.getSnapshot());

  useEffect(() => {
    // Subscribe to state changes
    const unsubscribe = playerState.subscribe(newState => {
      setState(newState);
    });

    return () => {
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    // Recalculate ETA periodically
    const interval = setInterval(() => {
      const eta = playerState.getQueueETA();
      setEta(eta);
    }, updateInterval);

    return () => clearInterval(interval);
  }, [updateInterval]);

  const etaStr = formatETA(etaSeconds);

  return (
    <Box flexDirection="column">
      <Text>
        Queue ETA: {etaStr} (at {state.currentSpeed.toFixed(2)}x)
      </Text>
      <Text dimColor>Episodes in queue: {state.queue.length}</Text>
    </Box>
  );
};
