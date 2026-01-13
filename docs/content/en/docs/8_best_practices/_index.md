---
title: "Best Practices"
type: docs
weight: 8
description: Best practices to follow when using porch
---

## Lorem Ipsum

This is the section where we describe the "Right way to use porch" if users are actively going against what we state are best practices support cannot be provided.

Examples:

- Recommendations for structuring packages, versions & variants
- How to design reusable templates/functions
- Performance / scaling tips (e.g. for large numbers of packages or functions)
- Operational guidance: monitoring, logging, health checks

FOR REPOSITORIES:

- THE USAGE OF MULTIPLE PORCH REPOS ON A SINGLE GIT REPO IS NOT RECOMMENDED FOR PORCH REPOS THAT PORCH WRITES PACKAGE REVISIONS TO AND SHOULD ONLY BE USED FOR READ ONLY UPSTREAM REPOS. THIS WILL BE SLOWER AND TAKE A PERFORMANCE HIT.
- THE OPTIMAL USE CASE IS A SINGLE PORCH REPO PER GIT REPO FOR EFFICIENCY SAKE.
