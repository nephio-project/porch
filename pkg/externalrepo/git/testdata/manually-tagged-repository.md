# Manually tagged repository

For easier orientation, an overview of the contents of the `manually-tagged-repository.tar`.

## Purpose

This repository contains a package with one porch created and one manually tagged package revision which also has a tag message.

## Contents

The repository in `manually-tagged-repository.tar` has the following contents
in `main` branch:

```
.
├── pkg
│   ├── Kptfile
│   ├── package-context.yaml
│   └── README.md
└── README.md
```

## Commits

The commit graph of the repository is:

```
* 66113e1 (HEAD -> main, tag: pkg/v2) manual v2
*   b0ae981 (tag: pkg/v1) Approve pkg/v1
|\  
| * 1a20d5d Intermediate commit: eval
| * ff12902 Intermediate commit: init
|/  
* 1250cd9 Initial commit
```

## Tags

```
pkg/v1 Approve pkg/v1
pkg/v2 some comment
```
