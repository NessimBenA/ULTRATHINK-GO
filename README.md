# ULTRATHINK-GO

Real-time agent feed monitor designed for Claude Code development workflows.

![ULTRATHINK Interface](https://img.shields.io/badge/Status-Active-green) ![Go Version](https://img.shields.io/badge/Go-1.22+-blue) ![License](https://img.shields.io/badge/License-MIT-yellow)

## Purpose

ULTRATHINK provides a live visual feed of what Claude Code is doing in your repository. Watch your AI assistant's progress in real-time with a clean, narrative-focused interface that tells the story of each action.

**Intent → Change → Result**

Every entry in the feed follows this simple pattern, making it easy to understand what the agent intended, what changed, and whether it succeeded.

## Features

- **Real-time Git Commit Monitoring** - See every commit as it happens with intent extraction
- **TODO.md File Watching** - Monitor Claude's task planning and progress updates
- **Interactive File Lists** - Click on multi-file commits to see exactly which files were modified
- **TODO.md Sidebar Viewer** - Click any TODO.md mention to view the current task list
- **Pause/Resume Controls** - Stop the feed when you need to focus
- **Event Filtering** - Show only commits, TODO updates, or everything
- **Current Objective Display** - Always see what Claude is working on right now

## Quick Start

### Prerequisites

- Go 1.22 or later
- Git repository (for monitoring commits)

### Installation

```bash
# Clone the repository
git clone https://github.com/NessimBenA/ULTRATHINK-GO.git
cd ULTRATHINK-GO

# Build the application
make build

# Run ULTRATHINK
make run
```

The interface will be available at `http://localhost:8080`

### Alternative Installation

```bash
# Install directly with go
go install github.com/NessimBenA/ULTRATHINK-GO@latest

# Run from anywhere
ultrathink
```

## Claude Code Integration

ULTRATHINK is specifically designed to work with Claude Code. To get the best experience:

1. **Add CLAUDE.md to your repository** - This file (included in this repo) configures Claude Code to maintain a TODO.md file and follow development patterns that ULTRATHINK can track

2. **Let Claude Code work** - As Claude Code makes commits and updates TODO.md, ULTRATHINK will show you exactly what's happening

3. **Monitor the feed** - Watch the Intent → Change → Result pattern for each action Claude takes

### Example Workflow

1. Start ULTRATHINK in your development repository: `ultrathink`
2. Open the web interface: `http://localhost:8080`
3. Ask Claude Code to implement a feature
4. Watch in real-time as Claude:
   - Updates TODO.md with implementation steps
   - Makes focused, single-file commits
   - Shows progress through the task list

## Configuration

### Environment Variables

- `PORT` - Server port (default: 8080)

Example:
```bash
PORT=3000 ultrathink
```

### Git Repository

ULTRATHINK automatically detects if you're in a Git repository. If not, it will still monitor TODO.md changes but won't show commit events.

## Architecture

- **Backend**: Go with Gorilla WebSocket for real-time communication
- **Frontend**: Vanilla JavaScript with clean, responsive design
- **Monitoring**: File system watchers and Git repository polling
- **Storage**: In-memory event buffer (latest 100 events)

## Development

### Building

```bash
# Development build
go build -o ultrathink main.go

# Run with custom port
PORT=8081 ./ultrathink
```

### Project Structure

```
├── main.go              # Core server and monitoring logic
├── static/
│   ├── index.html       # Single-page web interface
│   ├── app.js          # WebSocket client and UI logic
│   └── style.css       # Clean, dark theme styling
├── CLAUDE.md            # Claude Code configuration
├── TODO.md              # Task tracking (monitored by ULTRATHINK)
└── README.md           # This file
```

## Why ULTRATHINK?

When working with AI coding assistants like Claude Code, it's easy to lose track of what's happening. ULTRATHINK solves this by providing:

- **Transparency** - See exactly what the AI is doing and why
- **Confidence** - Understand the reasoning behind each change
- **Control** - Pause, filter, and review actions as they happen
- **Learning** - Understand AI development patterns and workflows

## Contributing

1. Fork the repository
2. Create a feature branch
3. Follow the single-file commit pattern (as Claude Code does)
4. Submit a pull request

## License

MIT License - see LICENSE file for details.

## Links

- [GitHub Repository](https://github.com/NessimBenA/ULTRATHINK-GO)
- [Issues & Feature Requests](https://github.com/NessimBenA/ULTRATHINK-GO/issues)