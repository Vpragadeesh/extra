use crossterm::event::{poll, read, Event, KeyCode};
use crossterm::terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen};
use crossterm::ExecutableCommand;
use ratatui::backend::CrosstermBackend;
use ratatui::layout::{Constraint, Direction, Layout};
use ratatui::style::{Modifier, Style};
use ratatui::widgets::{Block, Borders, List, ListItem, Paragraph, ListState};
use ratatui::Terminal;
use std::error::Error;
use std::fs;
use std::io::stdout;
use std::path::PathBuf;
use std::time::Duration;

fn main() -> Result<(), Box<dyn Error>> {
    // initialize terminal
    enable_raw_mode()?;
    let mut stdout = stdout();
    stdout.execute(EnterAlternateScreen)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let mut app = App::new()?;

    let res = run_app(&mut terminal, &mut app);

    // restore terminal
    disable_raw_mode()?;
    let stdout = terminal.backend_mut();
    stdout.execute(LeaveAlternateScreen)?;
    terminal.show_cursor()?;

    res
}

struct App {
    items: Vec<PathBuf>,
    selected: usize,
    queue: Vec<PathBuf>,
}

impl App {
    fn new() -> Result<Self, Box<dyn Error>> {
        let items = read_media_files(".")?;
        Ok(Self { items, selected: 0, queue: Vec::new() })
    }

    fn selected_item(&self) -> Option<&PathBuf> {
        self.items.get(self.selected)
    }
}

fn is_media(p: &PathBuf) -> bool {
    match p.extension().and_then(|s| s.to_str()) {
        Some(ext) => matches!(
            ext.to_lowercase().as_str(),
            "mp4" | "mov" | "mkv" | "avi" | "webm" | "m4v" | "mp3" | "flac"
        ),
        None => false,
    }
}

fn read_media_files(dir: &str) -> Result<Vec<PathBuf>, Box<dyn Error>> {
    let mut out = Vec::new();
    for entry in fs::read_dir(dir)? {
        let p = entry?.path();
        if p.is_file() && is_media(&p) {
            out.push(p);
        }
    }
    out.sort();
    Ok(out)
}

fn run_app(terminal: &mut Terminal<CrosstermBackend<std::io::Stdout>>, app: &mut App) -> Result<(), Box<dyn Error>> {
    loop {
        terminal.draw(|f| {
            let size = f.size();
            let chunks = Layout::default()
                .direction(Direction::Horizontal)
                .constraints([Constraint::Percentage(60), Constraint::Percentage(40)].as_ref())
                .split(size);

            // left: file list
            let items: Vec<ListItem> = app
                .items
                .iter()
                .map(|p| {
                    let name = p.file_name()
                        .map(|s| s.to_string_lossy().to_string())
                        .unwrap_or_else(|| p.display().to_string());
                    ListItem::new(name)
                })
                .collect();

            let list = List::new(items)
                .block(Block::default().borders(Borders::ALL).title("Media Files"))
                .highlight_style(Style::default().add_modifier(Modifier::REVERSED | Modifier::BOLD))
                .highlight_symbol(">> ");

            let mut list_state = ListState::default();
            list_state.select(Some(app.selected));
            f.render_stateful_widget(list, chunks[0], &mut list_state);

            // right: queue
            let qitems: Vec<ListItem> = app
                .queue
                .iter()
                .map(|p| {
                    let name = p.file_name()
                        .map(|s| s.to_string_lossy().to_string())
                        .unwrap_or_else(|| p.display().to_string());
                    ListItem::new(name)
                })
                .collect();

            let q = List::new(qitems).block(Block::default().borders(Borders::ALL).title("Queue"));
            f.render_widget(q, chunks[1]);

            // bottom help
            let help = Paragraph::new("↑/↓: Move  Enter: Queue  r: Refresh  c: Clear queue  q: Quit")
                .block(Block::default().borders(Borders::TOP));
            let help_area = Layout::default()
                .direction(Direction::Vertical)
                .constraints([Constraint::Min(0), Constraint::Length(1)].as_ref())
                .split(size);
            f.render_widget(help, help_area[1]);
        })?;

        if poll(Duration::from_millis(100))? {
            if let Event::Key(key) = read()? {
                match key.code {
                    KeyCode::Char('q') => return Ok(()),
                    KeyCode::Down => {
                        if !app.items.is_empty() {
                            app.selected = (app.selected + 1).min(app.items.len() - 1);
                        }
                    }
                    KeyCode::Up => {
                        if app.selected > 0 {
                            app.selected -= 1;
                        }
                    }
                    KeyCode::Enter => {
                        if let Some(p) = app.selected_item() {
                            app.queue.push(p.clone());
                        }
                    }
                    KeyCode::Char('r') => {
                        app.items = read_media_files(".")?;
                        app.selected = 0;
                    }
                    KeyCode::Char('c') => {
                        app.queue.clear();
                    }
                    _ => {}
                }
            }
        }
    }
}
