---
title: "Functionality"
type: docs
weight: 3
description: |
  Detailed operational behavior of the Repository Controller.
---

This section covers the operational behavior and runtime characteristics of the Repository Controller.

The controller manages repository synchronization through a combination of health checks and full sync operations. It implements intelligent scheduling, error-specific retry logic, and comprehensive status tracking to ensure reliable operation at scale.

Key aspects covered:

- How the controller decides when to sync repositories
- How different error types are handled with appropriate retry intervals
- What status information is maintained and how to interpret it
