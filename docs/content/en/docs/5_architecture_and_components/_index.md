---
title: "Architecture & Components"
type: docs
weight: 5
description: Porch's internal architecture and component design
---

This section provides detailed documentation of Porch's internal architecture, explaining how its components work together to manage KRM configuration packages.

## Overview

Porch is built as a Kubernetes extension API server with a modular architecture consisting of several key components:

![High Level Architecture](/static/images/porch/Porch-Architecture.drawio.svg)

## Core Components

### [Porch API Server]({{% relref "porch-apiserver" %}})

The Kubernetes extension API server that exposes Porch's API resources:
- Registers PackageRevision, PackageRevisionResources, and Package resources
- Handles CRUD operations and watch requests
- Integrates with Kubernetes RBAC for access control
- Manages background repository synchronization

### [Engine]({{% relref "engine" %}})

The Configuration as Data (CaD) Engine that orchestrates package operations:
- Manages package revision lifecycle (Draft → Proposed → Published)
- Coordinates task execution (init, clone, edit, upgrade)
- Enforces validation rules and business constraints
- Provides draft-commit workflow for package modifications

### [Package Cache]({{% relref "package-cache" %}})

The caching layer between the Engine and Git repositories:
- **CR Cache**: Stores metadata as Kubernetes custom resources
- **DB Cache**: Stores metadata in PostgreSQL for larger deployments
- Performs background synchronization with Git repositories
- Manages repository connections and lifecycle

### [Function Runner]({{% relref "function-runner" %}})

A standalone gRPC service for executing KRM functions:
- Runs functions in isolated pods or as executables
- Manages pod lifecycle with caching and garbage collection
- Handles image registry authentication and metadata
- Provides service mesh compatibility

### [Porch Controllers]({{% relref "controllers" %}})

Kubernetes controllers that automate package variant management:
- **PackageVariant Controller**: Creates and syncs downstream package variants
- **PackageVariantSet Controller**: Generates multiple PackageVariants from templates
- Detects upstream changes and creates upgrade/edit drafts
- Applies mutations and injections to generated packages

## Component Interactions

The components interact in a layered architecture:

1. **API Layer**: Porch API Server receives requests from Kubernetes clients
2. **Orchestration Layer**: Engine coordinates operations across components
3. **Storage Layer**: Package Cache manages repository data and synchronization
4. **Execution Layer**: Function Runner executes KRM functions in isolation
5. **Automation Layer**: Controllers provide declarative package management

## Design Principles

**Separation of Concerns**: Each component has a well-defined responsibility
- API Server: Kubernetes integration
- Engine: Business logic and orchestration
- Cache: Data management and synchronization
- Function Runner: Function execution isolation
- Controllers: Automation and templating

**Extensibility**: Components use interfaces and adapters
- Repository adapters support different storage backends (Git, OCI)
- Cache implementations (CR, DB) are interchangeable
- Function evaluators support multiple execution strategies

**Scalability**: Architecture supports horizontal scaling
- Function Runner runs as separate service
- DB Cache enables larger deployments
- Controllers can be scaled independently

**Reliability**: Built-in resilience mechanisms
- Background synchronization maintains consistency
- Draft-commit workflow ensures atomicity
- Watch streams provide real-time updates

## Exploring the Architecture

Each component section includes:
- **Overview**: What the component does and why it exists
- **Design**: Internal structure and key abstractions
- **Functionality**: Detailed behavior and algorithms
- **Interactions**: How it communicates with other components

Start with the [Porch API Server]({{% relref "porch-apiserver" %}}) to understand the entry point, then explore the [Engine]({{% relref "engine" %}}) for orchestration logic, and dive into specific components as needed.
