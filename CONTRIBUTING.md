# Contributing to PEPA

Thank you for your interest in contributing to PEPA! This document provides guidelines and instructions for contributing.

## Code of Conduct

Please be respectful and constructive in all interactions. We aim to maintain a welcoming and inclusive community.

## Development Setup

### Prerequisites

- Go 1.26 or later
- Node.js 22 or later
- Docker and Docker Compose
- PostgreSQL 18 (or use Docker)

### Getting Started

1. **Fork and clone the repository**
   ```bash
   git clone https://github.com/your-org/pepa.git
   cd pepa
   ```

2. **Install dependencies**
   ```bash
   # Go dependencies
   go mod download && go mod tidy

   # Frontend dependencies
   cd frontend && npm install
   ```

3. **Start infrastructure**
   ```bash
   make docker-up  # Starts PostgreSQL, Redis, MinIO
   ```

4. **Run the API server**
   ```bash
   make run-dev
   ```

5. **Run the frontend**
   ```bash
   cd frontend && npm run dev
   ```

## Making Changes

### Branch Naming

Use descriptive branch names:
- `feature/add-webhook-support`
- `fix/scorecard-evaluation-error`
- `docs/update-api-reference`

### Commit Messages

Write clear, concise commit messages:
```
feat(workflow): add retry logic for failed steps

- Implement exponential backoff with configurable max retries
- Add retry_count field to step_executions table
- Update workflow engine to handle transient failures
```

### Code Style

- **Go**: Follow standard Go conventions. Run `gofmt` and `go vet` before committing.
- **TypeScript/React**: Follow the existing patterns in the codebase. Use functional components with server components where possible.
- **SQL**: Use snake_case for identifiers. Add comments for complex queries.

### Testing

- Write tests for new functionality
- Ensure existing tests pass: `make test`
- Include integration tests for API endpoints

## Pull Request Process

1. Update documentation if needed
2. Add or update tests for your changes
3. Ensure the build passes: `make build && cd frontend && npm run build`
4. Submit a pull request against the `main` branch
5. Describe your changes clearly in the PR description

## Architecture Decisions

When making significant architectural changes:
1. Open an issue first to discuss the approach
2. Document the decision in the relevant `docs/` file
3. Consider backward compatibility

## Plugin Development

See `internal/plugin/sdk-go/` for the plugin SDK. Example plugins are in `plugins/examples/`.

## Reporting Issues

- Use the GitHub issue tracker
- Include steps to reproduce for bugs
- Specify your environment (OS, Go version, Docker version)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
