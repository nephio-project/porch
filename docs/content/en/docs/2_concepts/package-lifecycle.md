---
title: "Package Lifecycle"
type: docs
weight: 3
description:
---

## Package Lifecycle Workflow

Packages managed by Porch progress through several states, from creation to final publication. This workflow ensures that packages are reviewed and approved before they are published and consumed.

The typical lifecycle of a package is as follows:

1.  **Draft:** A user initializes a new package or clones an existing one. The package is in a `Draft` state, allowing the user to make changes freely in their local workspace.
2.  **Proposed:** Once the changes are ready for review, the user pushes the package, which transitions it to the `Proposed` state. In this stage, the package is available for review by other team members.
3.  **Review and Approval:**
    *   **Approved:** If the package is approved, it is ready to be published.
    *   **Rejected:** If changes are required, the package is rejected. The user must pull the package, make the necessary modifications, and re-propose it for another review.
4.  **Published:** After approval, the package is published. Published packages are considered stable and are available for deployment and consumption by other systems or clusters. They typically become the "latest" version of a package.

![Flowchart](/static/images/porch/flowchart.drawio.svg)
