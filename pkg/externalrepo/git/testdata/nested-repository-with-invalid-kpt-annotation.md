# Nested repository with invalid KPT annotation

For easier orientation, an overview of the contents of the `nested-repository-with-invalid-kpt-annotation.tar`.

## Purpose

This repository holds 3 packages (`bp1`, `bp2` and `bp10`) in 2 nested repositories (`blueprint1` and `blueprint2`),
each with multiple revisions.
Some of these revisions are manually tagged and one such tag points to a commit with an invalid KPT annotation.

Said invalid annotation:
```
kpt:{invalid:annotation}
```

## Contents

The repository in `nested-repository-with-invalid-kpt-annotation.tar` has the following contents
in `main` branch:

```
.
├── blueprint
│   ├── bp1
│   │   ├── Kptfile
│   │   ├── package-context.yaml
│   │   └── README.md
│   └── bp2
│       ├── Kptfile
│       ├── package-context.yaml
│       └── README.md
├── blueprint2
│   └── bp10
│       ├── Kptfile
│       ├── package-context.yaml
│       └── README.md
└── README.md
```

## Commits

The commit graph of the repository is:

```
* e18861a (HEAD -> main, tag: blueprint2/bp10/v1) manually add bp10
*   816906c (tag: blueprint/bp2/v2) Approve blueprint/bp2/v2
|\  
| * a1012a0 Intermediate commit: eval
| * 8618b8c Intermediate commit: patch
| * 3025e58 Intermediate commit: eval
| * 599620c Intermediate commit: edit
|/  
* 8a17645 (tag: blueprint/bp1/v3) valid annotation
* 1151aa6 (tag: blueprint/bp1/v2) invalid annotation
*   85d34e6 (tag: blueprint/bp2/v1) Approve blueprint/bp2/v1
|\  
| * ef6bbc4 (blueprint/bp2/v1) Intermediate commit: eval
| * 9fa7a52 Intermediate commit: init
* |   668b48e (tag: blueprint/bp1/v1) Approve blueprint/bp1/v1
|\ \  
| |/  
|/|   
| * 4ad2099 (blueprint/bp1/v1) Intermediate commit: eval
| * 0723d0a Intermediate commit: init
|/  
* 0674821 Initial commit
```

## Tags

```
blueprint/bp1/v1 Approve blueprint/bp1/v1
blueprint/bp1/v2 invalid annotation
blueprint/bp1/v3 valid annotation
blueprint/bp2/v1 Approve blueprint/bp2/v1
blueprint/bp2/v2 Approve blueprint/bp2/v2
blueprint2/bp10/v1 manually add bp10
```
