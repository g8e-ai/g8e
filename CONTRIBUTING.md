# Contributing to g8e

Hi! We're thrilled you want to help out with g8e. Whether you're fixing a bug, cleaning up code, or suggesting a new feature, we're happy to have you here.

## How to Help

We're building g8e with a mix of human creativity and AI assistance. That means there's always something to polish, refactor, or fix. Don't be shy—if you see something "smelly" or a bit weird, a PR to fix it is a top-tier contribution.

### Our Ground Rules

To keep the platform secure and solid, we follow a few core principles:
- **3-Layer Governance**: Every action is checked by hard gates (L1), agent consensus (L2), and usually you (L3).
- **Clean Breaks**: We don't do backwards compatibility. If it's broken, we fix it properly rather than adding shims.
- **Host-Native**: We run on the metal for speed and simplicity.

## Getting Started

1. **Fork & Clone**:
   ```bash
   git clone https://github.com/<your-username>/g8e.git && cd g8e
   ```
2. **Launch**: 
   ```bash
   ./g8e platform start
   ```
3. **Hack away**: Create a branch and start making changes!

## The Stack

- **Operator (Go)**: The heart of the platform. Handles storage and the event bus.
- **Engine (Python)**: The brains. Orchestrates AI agents and governance.
- **Dashboard (Node.js)**: The face. Our web UI and API gateway.

Everything lives in `./.g8e` (logs, data, etc.). You can edit files directly and restart the platform to see changes.

## Testing

We love tests! Make sure yours pass before sending a PR:
```bash
./g8e test g8eo   # Go
./g8e test g8ee   # Python
./g8e test client # Node.js
```

## Submitting a PR

1. Keep it focused (one change per PR is best).
2. Add a test if you're fixing a bug or adding a feature.
3. Use a clear prefix in your commit like `g8eo: fix the thing`.
4. We'll jump in to review as soon as we can!

## Get in Touch

Have questions? Email danny@g8e.ai. It's the fastest way to get help or talk shop.

## The Fine Print (CLA)

By contributing, you grant us a license to use your work in g8e (Apache 2.0). You still own your code, but you're giving us permission to build the platform with it. Thanks for helping us grow!
