# Contributing to lv

First off, thanks for taking the time to contribute!

The following is a set of guidelines for contributing to `lv`, which is hosted on GitHub. These are mostly guidelines, not rules. Use your best judgment, and feel free to propose changes to this document in a pull request.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [I Have a Question](#i-have-a-question)
- [I Want To Contribute](#i-want-to-contribute)
  - [Reporting Bugs](#reporting-bugs)
  - [Suggesting Enhancements](#suggesting-enhancements)
  - [Your First Code Contribution](#your-first-code-contribution)
  - [Pull Requests](#pull-requests)
- [Styleguides](#styleguides)
  - [Git Commit Messages](#git-commit-messages)

## Code of Conduct

This project and everyone participating in it is governed by a Code of Conduct. By participating, you are expected to uphold this code.

## I Have a Question

If you have questions, please search the existing [Issues](https://github.com/rajeshkannanramakrishnan/lv/issues) to see if someone else has asked the same thing. If you don't find an answer, feel free to open a new issue.

## I Want To Contribute

### Reporting Bugs

This section guides you through submitting a bug report for `lv`. Following these guidelines helps maintainers and the community understand your report, reproduce the behavior, and find related reports.

- **Use a clear and descriptive title** for the issue to identify the problem.
- **Describe the exact steps to reproduce the problem** in as many details as possible.
- **Provide specific examples to demonstrate the steps**. Include links to files or GitHub projects, or copy/pasteable snippets, which you use in those examples.
- **Describe the behavior you observed after following the steps** and point out what exactly is the problem with that behavior.
- **Explain which behavior you expected to see instead and why.**
- **Include screenshots and animated GIFs** which show you following the reproduction steps.

### Suggesting Enhancements

This section guides you through submitting an enhancement suggestion for `lv`, including completely new features and minor improvements to existing functionality.

- **Use a clear and descriptive title** for the issue to identify the suggestion.
- **Provide a step-by-step description of the suggested enhancement** in as many details as possible.
- **Explain why this enhancement would be useful** to most `lv` users.

### Your First Code Contribution

Unsure where to begin contributing to `lv`? You can look through these generic `help wanted` and `good first issue` labels:

- [Good first issue](https://github.com/rajeshkannanramakrishnan/lv/labels/good%20first%20issue) - these should only require a few lines of code, and a test or two.
- [Help wanted](https://github.com/rajeshkannanramakrishnan/lv/labels/help%20wanted) - issues which should be a bit more involved than `good first issue`.

### Pull Requests

The process described here has several goals:

- Maintain `lv`'s quality
- Fix problems that are important to users
- Engage the community in working toward the best possible `lv`
- Enable a sustainable system for `lv`'s maintainers to review contributions

Please follow these steps to have your contribution considered by the maintainers:

1. Follow all instructions in [the template](.github/PULL_REQUEST_TEMPLATE.md) (if available).
2. Follow the [styleguides](#styleguides)
3. After you submit your pull request, verify that all status checks are passing <details><summary>What if the status checks are failing?</summary>If a status check is failing, and you believe that the failure is unrelated to your change, please leave a comment on the pull request explaining why you believe the failure is unrelated. A maintainer will re-run the status check for you. If we conclude that the failure was a false positive, then we will open an issue to track that problem with our status check suite.</details>

## Styleguides

### Git Commit Messages

- Use the present tense ("Add feature" not "Added feature")
- Use the imperative mood ("Move cursor to..." not "Moves cursor to...")
- Limit the first line to 72 characters or less
- Reference issues and pull requests liberally after the first line

## Development Setup

`lv` uses [mise](https://mise.jdx.dev/) for managing development environments and tasks.

1.  **Prerequisites**:
    *   Install `mise`: https://mise.jdx.dev/getting-started.html
    *   Initialize mise (if not already done): `mise install`

2.  **Clone the repository**:
    ```bash
    git clone https://github.com/rajeshkannanramakrishnan/lv.git
    cd lv
    ```

3.  **Build**:
    We use `mise` to run build tasks defined in `mise.toml`.
    ```bash
    mise run build
    ```
    This generates the `lv` binary in the current directory.

4.  **Run Tests**:
    ```bash
    mise run test
    ```

5.  **Manual Build/Test (Alternative)**:
    If you don't want to use `mise`, you can use standard Go commands:
    ```bash
    go build -o lv main.go
    go test ./...
    ```

## Dependencies

`lv` uses the following key libraries:
- [Bubbletea](https://github.com/charmbracelet/bubbletea) for the TUI framework.
- [Lipgloss](https://github.com/charmbracelet/lipgloss) for styling.
- [Cobra](https://github.com/spf13/cobra) for CLI commands.

Happy coding!
