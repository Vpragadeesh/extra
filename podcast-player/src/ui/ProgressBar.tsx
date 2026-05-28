import React, { useEffect, useState } from 'react';
import { Box, Text } from 'ink';

interface ProgressBarProps {
  current: number; // seconds
  total: number; // seconds
  width?: number; // default 30
}

const formatTime = (seconds: number): string => {
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  if (hours > 0) {
    return `${hours}:${mins < 10 ? '0' : ''}${mins}:${secs < 10 ? '0' : ''}${secs}`;
  }

  return `${mins}:${secs < 10 ? '0' : ''}${secs}`;
};

export const ProgressBar: React.FC<ProgressBarProps> = ({ current, total, width = 30 }) => {
  const [displayCurrent, setDisplayCurrent] = useState(current);

  useEffect(() => {
    setDisplayCurrent(current);
  }, [current]);

  if (total === 0) {
    return (
      <Box flexDirection="column">
        <Text>{'░'.repeat(width)} 0:00 / 0:00 (0%)</Text>
      </Box>
    );
  }

  const percent = (displayCurrent / total) * 100;
  const filled = Math.floor((percent / 100) * width);
  const empty = width - filled;
  const bar = '█'.repeat(filled) + '░'.repeat(empty);

  const currentStr = formatTime(displayCurrent);
  const totalStr = formatTime(total);

  return (
    <Box flexDirection="column">
      <Text>[{bar}] {currentStr} / {totalStr} ({percent.toFixed(0)}%)</Text>
    </Box>
  );
};
