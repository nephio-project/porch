---
title: "Code Contribution"
type: docs
weight: 1
description: How to contribute to the Porch codebase
---

This guide explains how to set up your Git environment and contribute code to Porch.

## Fork the Repository

Porch uses a fork-based workflow. This protects the upstream repository from accidental changes and ensures all modifications go through the pull request review process.

1. Navigate to [github.com/nephio-project/porch](https://github.com/nephio-project/porch)
2. Click the **Fork** button in the top-right corner
3. Select your GitHub account as the destination

This creates a copy of the Porch repository under your GitHub account (e.g., `github.com/YOUR_USERNAME/porch`).

## Clone Your Fork

Clone your fork using SSH (required for pushing changes):

```bash
git clone git@github.com:YOUR_USERNAME/porch.git
cd porch
```

SSH authentication links your commits to your GitHub account, which is required for CLA verification. If you haven't set up SSH keys, see [GitHub's SSH documentation](https://docs.github.com/en/authentication/connecting-to-github-with-ssh).

## Configure Remotes

Add the upstream repository and prevent accidental pushes to it:

```bash
git remote add upstream https://github.com/nephio-project/porch.git
git remote set-url --push upstream no_push
```

Verify your remotes:

```bash
git remote -v
```

Expected output:
```
origin    git@github.com:YOUR_USERNAME/porch.git (fetch)
origin    git@github.com:YOUR_USERNAME/porch.git (push)
upstream  https://github.com/nephio-project/porch.git (fetch)
upstream  no_push (push)
```

## The Golden Rules

1. **Always update from upstream**, never from origin
2. **Always push to origin** (your fork), never to upstream
3. **Never commit directly to main branch** - always work on feature branches

## Keep Your Fork Updated

Before creating a new branch, sync your fork's main branch with upstream:

```bash
git checkout main
git fetch upstream
git rebase upstream/main
git push origin main
```

Do this regularly to keep your fork up to date.

## Create a Feature Branch

Create a branch for your changes:

```bash
git checkout -b feature-add-package-validation
```

Branch naming conventions:
- `feature-` - New features
- `fix-` - Bug fixes
- `refactor-` - Code refactoring
- `docs-` - Documentation changes

## Make Your Changes

1. Edit code, add tests, update documentation
2. Follow the [Development Environment]({{% relref "development-environment" %}}) guide to build and test locally
3. Ensure all checks and tests pass: `make check`, `make test` and `make test-e2e`
## Update Copyright on files

If you have added any new golang files, add [this golang copyright header](https://github.com/nephio-project/porch/blob/main/scripts/boilerplate.go.txt) to each new golang file you have added. If you have added any other files (Yaml, scripts, test data), add [this text copyright header](https://github.com/nephio-project/porch/blob/main/scripts/boilerplate.yaml.txt) file to each new file.

If you have updated existing files, amend the dates on the copyright notice. Assuming you are updating the code in 2026, use the following guide.

| ------------------- | ------------------------ |
| Existing dates      | New Dates                |
| ------------------- | ------------------------ |
| 2026                | 2026                     |
| 2025                | 2025-2026                |
| 2024                | 2024, 2026               |
| 2023                | 2023, 2026               |
| 2025-2026           | 2025-2026                |
| 2024-2026           | 2024-2026                |
| 2022-2026           | 2022-2026                |
| 2022-2025           | 2022-2026                |
| 2022-2024           | 2022-2024, 2026          |
| 2018-2020,2022-2025 | 2018-2020,2022-2026      |
| 2018-2020,2022-2024 | 2018-2020,2022-2024,2026 |
|         ...         |           ...            |


## Commit Your Changes

```bash
git add .
git commit -sm "feat: add package validation for lifecycle transitions"
```

Commit message format:
- `feat:` - New feature
- `fix:` - Bug fix
- `refactor:` - Code refactoring
- `test:` - Test changes
- `docs:` - Documentation
- `chore:` - Maintenance tasks

Use imperative mood ("add" not "added").

## Push to Your Fork

Push your branch to your fork (origin):

```bash
git push origin feature-add-package-validation
```

## Create a Pull Request

1. Navigate to [github.com/nephio-project/porch](https://github.com/nephio-project/porch)
2. GitHub will show a prompt to create a PR from your recently pushed branch
3. Click **Compare & pull request**
4. Fill in the PR template:
   - Clear title describing the change
   - Description explaining what, why, and how
   - Reference related issues: "Fixes #123"
   - Include test results if applicable

The EasyCLA bot will prompt you to sign the CLA if you haven't already (see [Before You Start]({{% relref "../" %}})).

## Update Your PR

If maintainers request changes:

1. Make the requested changes on your local branch
2. Commit the changes: `git commit -sm "fix: address review feedback"`
3. Push to your fork: `git push origin feature-add-package-validation`

The PR will automatically update with your new commits.

## Rebase on Latest Main

If your PR becomes outdated with the main branch:

```bash
git checkout feature-add-package-validation
git fetch upstream
git rebase upstream/main
git push origin feature-add-package-validation --force-with-lease
```

Use `--force-with-lease` instead of `--force` for safer force-pushing.

## After Your PR is Merged

Clean up your local and remote branches:

```bash
git checkout main
git fetch upstream
git rebase upstream/main
git push origin main
git branch -d feature-add-package-validation
git push origin --delete feature-add-package-validation
```

## Common Git Operations

### Switch Between Branches

```bash
git checkout main
git checkout feature/my-feature
```

### View Branch Status

```bash
git status
git branch -a  # List all branches
```

### Discard Local Changes

```bash
git checkout -- <file>  # Discard changes to specific file
git reset --hard        # Discard all local changes
```

### View Commit History

```bash
git log --oneline
git log --graph --oneline --all
```

## Getting Help

If you encounter Git issues:
- Check existing GitHub issues
- Ask in the Nephio Slack #porch channel
- Consult [GitHub documentation](https://docs.github.com)
