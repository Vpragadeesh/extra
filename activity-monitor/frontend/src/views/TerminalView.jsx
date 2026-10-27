import { Terminal as TerminalIcon } from 'lucide-react';
import Timeline from '../components/Timeline';

export default function TerminalView({ events, loading }) {
  const terminalEvents = events.filter((event) => event.source === 'fish').slice(0, 30);

  return (
    <div className="mt-6">
      <Timeline
        icon={TerminalIcon}
        title="Terminal commands"
        detail="fish first-token events"
        id="terminal-commands"
        events={terminalEvents}
        loading={loading}
        emptyText="No fish commands recorded for this day."
        emptyAction="Source fish/activity-monitor.fish from your config.fish, then run any command — only its first token is stored."
      />
    </div>
  );
}
