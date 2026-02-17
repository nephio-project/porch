---
title: "Deployments"
type: docs
weight: 2
description: "Deploy Porch in different environments"
---

This section covers deploying Porch in various environments, from local development to production clusters.

## Deployment Methods

### [Local Development Environment]({{% relref "local-dev-env-deployment" %}})
Set up Porch locally using Kind for development and testing. This includes:
- Automated Kind cluster setup
- Built-in Gitea for Git repositories
- Choice between CR cache or database cache
- Pre-configured development tools

### [Catalog Deployment]({{% relref "catalog-deployment" %}})
Deploy Porch in production environments using the Nephio catalog:
- Production-ready configuration
- Integration with existing Kubernetes clusters
- Scalable deployment options

## Choosing Your Deployment

- **Use Local Development** when:
  - Developing or testing Porch functionality
  - Learning Porch concepts
  - Contributing to Porch development

- **Use Catalog Deployment** when:
  - Deploying to production environments
  - Integrating with existing infrastructure
  - Requiring enterprise-grade reliability