# Contributing to g8ee

g8ee is a first-party component of the g8e platform. This document provides guidelines and instructions for contributing to the ensemble. For platform-wide contribution guidelines, see the repo-root [CONTRIBUTING.md](../.github/CONTRIBUTING.md).

## Code of Conduct

Be respectful, constructive, and collaborative. We're here to build something great together.

## Getting Started

### Prerequisites

- Python 3.12+ (per `pyproject.toml`; the `requires-python` field is the source of truth)
- Git
- Virtual environment (recommended)

### Setup Development Environment

The ensemble depends on the in-tree `protocol/python/` g8e package. Install the protocol package first, then the ensemble in editable mode:

```bash
# From the g8e repo root
python3 -m venv .venv
source .venv/bin/activate  # On Windows: .venv\Scripts\activate

# Install the in-tree g8e protocol package first
pip install protocol/python

# Install the ensemble with dev/test extras
pip install -e "ensemble[dev,test]"

# Copy environment template and configure
cp ensemble/.env.example ensemble/.env
# Edit ensemble/.env with your configuration
```

### Development Workflow

1. Create a new branch for your feature or bugfix
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bugfix-name
   ```

2. Make your changes following the coding standards

3. Run tests and linting
   ```bash
   # Format code
   ruff format .

   # Lint code
   ruff check .

   # Type check
   pyright

   # Run tests
   pytest tests/ -v -m "not ai_integration and not requires_web_search and not requires_api and not e2e"
   ```

4. Commit your changes with clear, descriptive messages
   ```bash
   git add .
   git commit -m "feat: add your feature description"
   ```

5. Push and create a pull request
   ```bash
   git push origin feature/your-feature-name
   ```

## Coding Standards

### Code Style

- Follow PEP 8 guidelines
- Use Ruff for formatting and linting
- Maximum line length: 100 characters
- Use type hints for all function signatures
- Write docstrings for all public functions and classes

### Type Checking

- Use Pyright for static type checking
- All new code must pass type checking without errors
- Use `from __future__ import annotations` for forward references

### Testing

- Write unit tests for all new functionality
- Use pytest as the test framework
- Aim for high test coverage
- Use descriptive test names
- Mock external dependencies (APIs, databases, etc.)

### Commit Messages

Follow conventional commit format:
- `feat:` for new features
- `fix:` for bug fixes
- `docs:` for documentation changes
- `style:` for code style changes (formatting, etc.)
- `refactor:` for code refactoring
- `test:` for adding or updating tests
- `chore:` for maintenance tasks

Examples:
```
feat: add support for new LLM provider
fix: resolve memory leak in connection pool
docs: update API documentation
test: add integration tests for auth service
```

## Project Structure

```
ensemble/
├── app/                 # Main application code
│   ├── clients/         # External service clients
│   ├── constants/       # Application constants
│   ├── db/              # Database models and operations
│   ├── llm/             # LLM provider implementations
│   ├── models/          # Pydantic models
│   ├── protocol/        # Protocol definitions
│   ├── routers/         # FastAPI route handlers
│   ├── services/        # Business logic services
│   └── utils/           # Utility functions
├── tests/               # Unit and integration tests (Tier 1, co-located)
├── evals/               # Evaluation suite
├── config/              # Configuration files
├── scripts/             # Utility scripts
├── pyproject.toml       # Project configuration
├── Dockerfile           # Docker image (build context: g8e repo root)
└── README.md            # Project readme
```

## Pull Request Guidelines

### Before Submitting

- Ensure all tests pass
- Run linting and type checking
- Update documentation if needed
- Add tests for new functionality
- Rebase your branch on the latest main if needed

### PR Description

Include:
- Clear title describing the change
- Description of what the PR does
- Why the change is needed
- Any breaking changes
- Related issues (if any)
- Screenshots for UI changes (if applicable)

### Review Process

- Maintainers will review your PR
- Address feedback in a timely manner
- Keep discussions focused and constructive
- Update the PR based on review comments

## Testing Guidelines

### Unit Tests

- Test individual functions and classes in isolation
- Mock external dependencies
- Use pytest fixtures for common test setup
- Mark slow tests with `@pytest.mark.slow`

### Integration Tests

- Test interactions between components
- May require external services (databases, APIs)
- Mark with `@pytest.mark.integration`
- Use test markers to categorize tests

### Test Markers

Available pytest markers:
- `unit`: Unit tests (fast, isolated)
- `integration`: Integration tests (may require external services)
- `ai_integration`: AI integration tests requiring real LLM API calls
- `slow`: Tests that take longer than 1 second
- `requires_web_search`: Tests requiring web search configuration
- `e2e`: End-to-end tests
- `smoke`: Quick smoke tests for CI

Run specific test categories:
```bash
pytest tests/ -m unit
pytest tests/ -m integration
pytest tests/ -m "not slow"
```

## Documentation

- Update README.md for user-facing changes
- Add docstrings to all public functions and classes
- Keep documentation in sync with code changes
- Use clear, concise language

## Questions or Issues?

- Open an issue for bugs or feature requests
- Use discussions for questions and ideas
- Check existing issues before creating new ones

## License

By contributing to g8ee, you agree that your contributions will be licensed under the Business Source License 1.1 (BSL 1.1), which converts to Apache License 2.0 on 2030-08-18. See the [LICENSE](LICENSE) file in this directory and the repo-root [LICENSE](../LICENSE) file for details.
