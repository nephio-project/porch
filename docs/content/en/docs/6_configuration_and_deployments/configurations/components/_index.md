---
title: "Components"
type: docs
weight: 1
description: "Configure individual Porch components"
---

Configure each Porch component individually for optimal performance and security.

## Core Components

### [Porch Server]({{% relref "porch-server-config" %}})
The main API server that handles package operations and Git repository interactions:
- [Git Authentication]({{% relref "porch-server-config/git-authentication" %}}) - Repository authentication methods
- [Cert Manager Webhooks]({{% relref "porch-server-config/cert-manager-webhooks" %}}) - Webhook certificate management
### [Porch Controllers]({{% relref "porch-controllers-config" %}})
Controllers that manage the lifecycle of PackageRevisions, PackageVariants, and other Porch resources.

### [Function Runner]({{% relref "function-runner-config" %}})
Configure the KPT function execution environment:
- Function registry settings
- Security policies
- Resource limits
- Scaling configuration
- [Private Registry Access]({{% relref "function-runner-config/private-registries-config" %}})